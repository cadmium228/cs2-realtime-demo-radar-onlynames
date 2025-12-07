package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	imgcolor "image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fogleman/gg"
	"github.com/golang/geo/r3"
	ex "github.com/markus-wa/demoinfocs-golang/v5/examples"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"golang.org/x/sys/windows/registry"
)

const DOT_SIZE = 14

type PlayerData struct {
	Name       string `json:"name"`
	Health     int    `json:"health"`
	Team       int    `json:"team"`
	IsAlive    bool   `json:"isAlive"`
	UserID     int    `json:"userId"`
}

var (
	lastMapImg   []byte
	playersData  []PlayerData
	demoPath     string
	mapName      string
	initialTime  time.Time
	highlightID  int

	cyan    = color.New(color.FgCyan, color.Bold)
	green   = color.New(color.FgGreen, color.Bold)
	yellow  = color.New(color.FgYellow)
	red     = color.New(color.FgRed, color.Bold)
	magenta = color.New(color.FgMagenta, color.Bold)
	blue    = color.New(color.FgBlue)
	white   = color.New(color.FgWhite)
)

func main() {
	cs2Path := detectCS2Path()
	reader := bufio.NewReader(os.Stdin)

	if cs2Path != "" {
		green.Printf("✓ CS2 detected: %s\n\n", cs2Path)
		yellow.Print("Use this path? (Y/n): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "n" || response == "no" {
			cs2Path = ""
		}
	}

	if cs2Path == "" {
		yellow.Print("Enter CS2 path manually: ")
		cs2Path, _ = reader.ReadString('\n')
		cs2Path = strings.TrimSpace(cs2Path)
	}

	cs2Path = strings.ReplaceAll(cs2Path, "\\", "/")
	cs2Path = strings.ReplaceAll(cs2Path, "\"", "")

	cyan.Print("\nDemo filename (e.g. 'radar' or 'radar.dem'): ")
	demoName, _ := reader.ReadString('\n')
	demoName = strings.TrimSpace(demoName)

	if !strings.HasSuffix(strings.ToLower(demoName), ".dem") {
		demoName += ".dem"
	}

	demoPath = filepath.Join(cs2Path, demoName)
	demoPath = strings.ReplaceAll(demoPath, "\\", "/")

	cyan.Print("Map name (e.g. 'de_mirage'): ")
	mapNameInput, _ := reader.ReadString('\n')
	mapName = strings.TrimSpace(mapNameInput)
	if mapName == "" {
		mapName = "de_mirage"
	}

	fmt.Println()
	blue.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	white.Printf("  Demo: %s\n", demoPath)
	white.Printf("  Map:  %s\n", mapName)
	blue.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	contents, _ := os.ReadFile("./index.html")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(contents))
	})
	
	http.HandleFunc("/select", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.URL.Query().Get("id")
		if id, err := strconv.Atoi(idStr); err == nil {
			highlightID = id
		}
	})

	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(lastMapImg)
	})
	http.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(playersData)
	})

	go http.ListenAndServe(":8080", nil)

	green.Println("✓ Server started at http://localhost:8080")
	openBrowser("http://localhost:8080")
	yellow.Println("⏳ Waiting for demo file updates...")
	fmt.Println()

	fileInfo, err := os.Stat(demoPath)
	if err != nil {
		initialTime = time.Now()
	} else {
		initialTime = fileInfo.ModTime()
	}

	for {
		fileInfo, err := os.Stat(demoPath)
		if err != nil {
			time.Sleep(50 * time.Millisecond) 
			continue
		}

		if fileInfo.ModTime().After(initialTime) && fileInfo.Size() > 0 {
			initialTime = fileInfo.ModTime()
			f, err := os.Open(demoPath)
			if err != nil {
				red.Printf("✗ Failed to open: %s\n", err)
			} else {
				magenta.Printf("⟳ Processing [%s]\n", time.Now().Format("15:04:05"))
				processDemo(f)
				f.Close()
			}
		}
		
		time.Sleep(50 * time.Millisecond)
	}
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		red.Println("Could not open browser automatically")
	}
}

func detectCS2Path() string {
	steamPath := getSteamPathFromRegistry()
	if steamPath != "" {
		cs2Path := filepath.Join(steamPath, "steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo")
		if _, err := os.Stat(cs2Path); err == nil {
			return cs2Path
		}
		libraryFoldersPath := filepath.Join(steamPath, "steamapps", "libraryfolders.vdf")
		if additionalPaths := parseLibraryFolders(libraryFoldersPath); len(additionalPaths) > 0 {
			for _, libPath := range additionalPaths {
				cs2Path := filepath.Join(libPath, "steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo")
				if _, err := os.Stat(cs2Path); err == nil {
					return cs2Path
				}
			}
		}
	}
	possiblePaths := []string{
		"C:/Program Files (x86)/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"D:/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"E:/Steam/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"C:/SteamLibrary/steamapps/common/Counter-Strike Global Offensive/game/csgo",
		"D:/SteamLibrary/steamapps/common/Counter-Strike Global Offensive/game/csgo",
	}
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func getSteamPathFromRegistry() string {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Valve\Steam`, registry.QUERY_VALUE)
	if err != nil {
		k, err = registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`, registry.QUERY_VALUE)
		if err != nil {
			return ""
		}
	}
	defer k.Close()
	steamPath, _, err := k.GetStringValue("SteamPath")
	if err != nil {
		return ""
	}
	steamPath = strings.ReplaceAll(steamPath, "\\", "/")
	return steamPath
}

func parseLibraryFolders(vdfPath string) []string {
	var paths []string
	content, err := os.ReadFile(vdfPath)
	if err != nil {
		return paths
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "\"path\"") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 4 {
				path := parts[3]
				path = strings.ReplaceAll(path, "\\\\", "/")
				path = strings.ReplaceAll(path, "\\", "/")
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func processDemo(f *os.File) {
	defer func() {
		if r := recover(); r != nil {
		}
	}()

	p := demoinfocs.NewParser(f)

	var (
		mapMetadata ex.Map      = ex.GetMapMetadata(mapName)
		mapRadarImg image.Image = ex.GetMapRadar(mapName)
	)

	err := p.ParseToEnd()
	if err != nil {
	}

	if mapRadarImg == nil {
		return
	}

	dc := gg.NewContextForImage(mapRadarImg)
	playersData = []PlayerData{}

	bomb := p.GameState().Bomb()
	var bombPos *r3.Vector
	if bomb != nil && bomb.Carrier == nil {
		pos := bomb.Position()
		bombPos = &pos
	}

	for _, player := range p.GameState().Participants().Playing() {
		pos := player.Position()
		x, y := mapMetadata.TranslateScale(pos.X, pos.Y)

		hp := player.Health()
		name := player.Name
		isAlive := player.IsAlive()

		playersData = append(playersData, PlayerData{
			Name:       name,
			Health:     hp,
			Team:       int(player.Team),
			IsAlive:    isAlive,
			UserID:     player.UserID,
		})

		if !isAlive {
			continue
		}

		if player.UserID == highlightID {
			dc.SetRGBA(1, 1, 0, 0.4)
			dc.DrawCircle(x, y, 35)
			dc.Fill()
			
			dc.SetRGBA(1, 1, 0, 0.8)
			dc.SetLineWidth(2)
			dc.DrawCircle(x, y, 30)
			dc.Stroke()
		}

		var col imgcolor.RGBA
		switch player.Team {
		case 2:
			col = imgcolor.RGBA{220, 60, 60, 255}
		case 3:
			col = imgcolor.RGBA{60, 100, 220, 255}
		}

		dc.SetRGBA(0, 0, 0, 0.8)
		dc.DrawCircle(x, y, DOT_SIZE/2+1)
		dc.Fill()

		dc.SetRGBA(float64(col.R)/255, float64(col.G)/255, float64(col.B)/255, 1)
		dc.DrawCircle(x, y, DOT_SIZE/2)
		dc.Fill()

		dc.LoadFontFace("C:/Windows/Fonts/arialbd.ttf", 11) 
		
		nameText := name
		if len(nameText) > 12 {
			nameText = nameText[:12] + ".."
		}
		
		textWidth, _ := dc.MeasureString(nameText)

		dc.SetRGBA(0, 0, 0, 1)
		dc.DrawString(nameText, x-textWidth/2+1, y-DOT_SIZE)
		dc.DrawString(nameText, x-textWidth/2-1, y-DOT_SIZE)
		dc.DrawString(nameText, x-textWidth/2, y-DOT_SIZE+1)
		dc.DrawString(nameText, x-textWidth/2, y-DOT_SIZE-1)

		if player.UserID == highlightID {
			dc.SetRGBA(1, 1, 0, 1) 
		} else {
			dc.SetRGBA(1, 1, 1, 1)
		}
		dc.DrawString(nameText, x-textWidth/2, y-DOT_SIZE)
	}

	if bombPos != nil {
		bx, by := mapMetadata.TranslateScale(bombPos.X, bombPos.Y)
		dc.SetRGBA(1, 0, 0, 0.5)
		dc.DrawCircle(bx, by, 12)
		dc.Fill()
		dc.SetRGBA(1, 0, 0, 1)
		dc.DrawCircle(bx, by, 6)
		dc.Fill()
		dc.SetRGBA(1, 1, 1, 1)
		dc.LoadFontFace("C:/Windows/Fonts/arialbd.ttf", 12)
		dc.DrawString("C4", bx-8, by+4)
	}

	sort.SliceStable(playersData, func(i, j int) bool {
		if playersData[i].Team != playersData[j].Team {
			return playersData[i].Team < playersData[j].Team
		}
		return playersData[i].UserID < playersData[j].UserID
	})

	buffer := bytes.NewBuffer(nil)
	png.Encode(buffer, dc.Image())
	lastMapImg = buffer.Bytes()
}