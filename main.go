package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed yt-dlp.exe
var ytdlpBytes []byte

//go:embed ffmpeg.exe
var ffmpegBytes []byte

type LogEntry struct {
	ID    int    `json:"id"`
	Date  string `json:"date"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Структура для десериализации метаданных из yt-dlp
type VideoMetadata struct {
	Title string `json:"title"`
}

const jsonFile = "history.json"

func readHistory() []LogEntry {
	var history []LogEntry
	file, err := os.ReadFile(jsonFile)
	if err != nil {
		return history
	}
	_ = json.Unmarshal(file, &history)
	return history
}

func appendHistory(title, url string) {
	history := readHistory()

	newID := 1
	if len(history) > 0 {
		newID = history[len(history)-1].ID + 1
	}

	newEntry := LogEntry{
		ID:    newID,
		Date:  time.Now().Format("2006-01-02 15:04:05"),
		Title: title,
		URL:   url,
	}

	history = append(history, newEntry)
	data, _ := json.MarshalIndent(history, "", "  ")
	_ = os.WriteFile(jsonFile, data, 0644)
}

// Парсинг метаданных через JSON-дамп для полной поддержки кириллицы
func downloadAndGetTitle(ytdlpPath, url string) (string, error) {
	// Безопасный запрос метаданных в формате JSON
	cmdTitle := exec.Command(ytdlpPath, "--dump-json", url)
	var outTitle bytes.Buffer
	cmdTitle.Stdout = &outTitle

	title := "Unknown YouTube Video"
	if err := cmdTitle.Run(); err == nil {
		var meta VideoMetadata
		if err := json.Unmarshal(outTitle.Bytes(), &meta); err == nil && meta.Title != "" {
			title = meta.Title
		}
	}

	// Инициализация основного процесса загрузки
	cmd := exec.Command(ytdlpPath,
		"-f", "bv*[ext=mp4]+ba[ext=m4a]/bv*+ba/best",
		"--merge-output-format", "mp4",
		"-o", "Downloads/%(title)s.%(ext)s",
		url,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	return title, err
}

func main() {
	tempDir := filepath.Join(os.TempDir(), "phonker_loader")
	_ = os.MkdirAll(tempDir, os.ModePerm)

	ytdlpPath := filepath.Join(tempDir, "yt-dlp.exe")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg.exe")

	_ = os.WriteFile(ytdlpPath, ytdlpBytes, 0755)
	_ = os.WriteFile(ffmpegPath, ffmpegBytes, 0755)

	defer os.Remove(ytdlpPath)
	defer os.Remove(ffmpegPath)

	for {
		fmt.Println("\n=======================================")
		fmt.Println("    YOUTUBE DOWNLOADER BY PHONKER     ")
		fmt.Println("=======================================")
		fmt.Println("[1] Download Video")
		fmt.Println("[2] Open Downloads Directory")
		fmt.Println("[3] Show Download History")
		fmt.Println("[4] Exit")
		fmt.Println("=======================================")

		var choice string
		fmt.Print("Enter option (1-4): ")
		fmt.Scanln(&choice)
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			var url string
			fmt.Print("\nEnter YouTube URL: ")
			fmt.Scanln(&url)
			url = strings.TrimSpace(url)

			if url == "" {
				fmt.Println("Error: URL cannot be empty.")
				continue
			}

			fmt.Println("\n[INFO] Fetching video metadata via JSON dump...")
			videoTitle, err := downloadAndGetTitle(ytdlpPath, url)

			if err != nil {
				fmt.Printf("\n[ERROR] Task failed: %v\n", err)
			} else {
				fmt.Printf("[INFO] Target resolved successfully.\n")
				fmt.Println("[SUCCESS] Download completed.")
				appendHistory(videoTitle, url)
			}

		case "2":
			downloadsPath := "Downloads"
			_ = os.MkdirAll(downloadsPath, os.ModePerm)
			err := exec.Command("explorer", downloadsPath).Start()
			if err != nil {
				fmt.Printf("[ERROR] Failed to open directory: %v\n", err)
			} else {
				fmt.Println("[INFO] Opening Downloads folder...")
			}

		case "3":
			fmt.Println("\n=== DOWNLOAD HISTORY ===")
			history := readHistory()
			if len(history) == 0 {
				fmt.Println("History log is empty.")
			} else {
				for i := len(history) - 1; i >= 0; i-- {
					fmt.Printf("[%d] %s\n    Title: %s\n    URL:   %s\n\n",
						history[i].ID, history[i].Date, history[i].Title, history[i].URL)
				}
			}
			fmt.Println("========================")

		case "4":
			fmt.Println("\nTerminating process...")
			return

		default:
			fmt.Println("[ERROR] Invalid option selected.")
		}
	}
}
