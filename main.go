package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	ex "github.com/markus-wa/demoinfocs-golang/v5/examples"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"golang.org/x/sys/windows/registry"
)

type Config struct {
	CS2Path      string `json:"cs2_path"`
	DemoPath     string `json:"demo_path"`
	MapName      string `json:"map_name"`
	CustomMapImg string `json:"custom_map_img"`
}

type PlayerData struct {
	Name    string  `json:"name"`
	Health  int     `json:"health"`
	Team    int     `json:"team"`
	IsAlive bool    `json:"isAlive"`
	UserID  int     `json:"userId"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
}

type BombData struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type GameData struct {
	Players []PlayerData `json:"players"`
	Bomb    *BombData    `json:"bomb"`
}

var (
	backgroundImg []byte
	currentGame   GameData
	gameMutex     sync.RWMutex
	appConfig     Config
	configFile    = "config.json"

	cyan   = color.New(color.FgCyan, color.Bold)
	green  = color.New(color.FgGreen, color.Bold)
	yellow = color.New(color.FgYellow)
	red    = color.New(color.FgRed, color.Bold)
	blue   = color.New(color.FgBlue)
	white  = color.New(color.FgWhite)
)

type TailReader struct {
	f *os.File
}

func (t TailReader) Read(b []byte) (int, error) {
	for {
		n, err := t.f.Read(b)
		if n > 0 {
			return n, nil
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
		time.Sleep(1 * time.Millisecond)
	}
}

func loadConfig() {
	if _, err := os.Stat(configFile); err == nil {
		data, err := os.ReadFile(configFile)
		if err == nil {
			json.Unmarshal(data, &appConfig)
		}
	}
}

func saveConfig() {
	data, _ := json.MarshalIndent(appConfig, "", "  ")
	os.WriteFile(configFile, data, 0644)
}

func prompt(label string, defaultValue string, reader *bufio.Reader) string {
	if defaultValue != "" {
		cyan.Printf("%s [%s]: ", label, defaultValue)
	} else {
		cyan.Printf("%s: ", label)
	}

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func main() {
	loadConfig()
	reader := bufio.NewReader(os.Stdin)

	detectedPath := detectCS2Path()
	defaultCS2 := appConfig.CS2Path
	if defaultCS2 == "" {
		defaultCS2 = detectedPath
	}

	appConfig.CS2Path = prompt("CS2 Path", defaultCS2, reader)
	appConfig.CS2Path = strings.ReplaceAll(appConfig.CS2Path, "\\", "/")
	appConfig.CS2Path = strings.ReplaceAll(appConfig.CS2Path, "\"", "")

	appConfig.DemoPath = prompt("Demo File (e.g. radar.dem)", appConfig.DemoPath, reader)
	if !strings.HasSuffix(strings.ToLower(appConfig.DemoPath), ".dem") {
		appConfig.DemoPath += ".dem"
	}

	fullDemoPath := filepath.Join(appConfig.CS2Path, appConfig.DemoPath)
	fullDemoPath = strings.ReplaceAll(fullDemoPath, "\\", "/")

	defaultMap := appConfig.MapName
	if defaultMap == "" {
		defaultMap = "de_mirage"
	}
	appConfig.MapName = prompt("Map Name (e.g. de_mirage)", defaultMap, reader)

	appConfig.CustomMapImg = prompt("Custom Map Image Path (optional, empty for default)", appConfig.CustomMapImg, reader)
	appConfig.CustomMapImg = strings.ReplaceAll(appConfig.CustomMapImg, "\\", "/")
	appConfig.CustomMapImg = strings.ReplaceAll(appConfig.CustomMapImg, "\"", "")

	saveConfig()

	fmt.Println()
	blue.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	white.Printf("  Demo: %s\n", fullDemoPath)
	white.Printf("  Map:  %s\n", appConfig.MapName)
	if appConfig.CustomMapImg != "" {
		white.Printf("  Img:  %s\n", appConfig.CustomMapImg)
	}
	blue.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println()

	prepareMapBackground()

	contents, _ := os.ReadFile("./index.html")
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(contents))
	})

	http.HandleFunc("/map", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(backgroundImg)
	})

	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		gameMutex.RLock()
		json.NewEncoder(w).Encode(currentGame)
		gameMutex.RUnlock()
	})

	go http.ListenAndServe(":8080", nil)

	green.Println("✓ Server started at http://localhost:8080")
	openBrowser("http://localhost:8080")
	yellow.Println("⏳ Waiting for demo file...")

	for {
		if _, err := os.Stat(fullDemoPath); err == nil {
			green.Println("✓ File found! Starting live parser...")
			startLiveParser(fullDemoPath)
			break
		}
		time.Sleep(1 * time.Second)
	}

	select {}
}

func startLiveParser(path string) {
	f, err := os.Open(path)
	if err != nil {
		red.Println("Error opening file:", err)
		return
	}

	go func() {
		defer f.Close()

		tailReader := TailReader{f: f}
		p := demoinfocs.NewParser(tailReader)
		defer p.Close()

		var mapMetadata ex.Map = ex.GetMapMetadata(appConfig.MapName)
		mapImg := ex.GetMapRadar(appConfig.MapName)

		width := 1024.0
		height := 1024.0

		if mapImg != nil {
			width = float64(mapImg.Bounds().Dx())
			height = float64(mapImg.Bounds().Dy())
		}

		for {
			more, err := p.ParseNextFrame()
			if err != nil {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			if !more {
				time.Sleep(1 * time.Millisecond)
				continue
			}

			var players []PlayerData
			var bombData *BombData

			bomb := p.GameState().Bomb()
			if bomb != nil && bomb.Carrier == nil {
				pos := bomb.Position()
				bx, by := mapMetadata.TranslateScale(pos.X, pos.Y)
				bombData = &BombData{
					X: (bx / width) * 100,
					Y: (by / height) * 100,
				}
			}

			for _, player := range p.GameState().Participants().Playing() {
				if !player.IsAlive() {
					continue
				}

				pos := player.Position()
				x, y := mapMetadata.TranslateScale(pos.X, pos.Y)

				players = append(players, PlayerData{
					Name:    player.Name,
					Health:  player.Health(),
					Team:    int(player.Team),
					IsAlive: true,
					UserID:  player.UserID,
					X:       (x / width) * 100,
					Y:       (y / height) * 100,
				})
			}

			sort.SliceStable(players, func(i, j int) bool {
				if players[i].Team != players[j].Team {
					return players[i].Team < players[j].Team
				}
				return players[i].UserID < players[j].UserID
			})

			gameMutex.Lock()
			currentGame = GameData{
				Players: players,
				Bomb:    bombData,
			}
			gameMutex.Unlock()
		}
	}()
}

func prepareMapBackground() {
	var imgBytes []byte

	if appConfig.CustomMapImg != "" {
		data, err := os.ReadFile(appConfig.CustomMapImg)
		if err == nil {
			imgBytes = data
			green.Println("✓ Loaded custom map image")
		} else {
			red.Println("✗ Failed to load custom image, using default")
		}
	}

	if imgBytes == nil {
		mapRadarImg := ex.GetMapRadar(appConfig.MapName)
		if mapRadarImg == nil {
			red.Println("Error: Default map image not found!")
			return
		}
		buffer := bytes.NewBuffer(nil)
		png.Encode(buffer, mapRadarImg)
		imgBytes = buffer.Bytes()
	}

	backgroundImg = imgBytes
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
