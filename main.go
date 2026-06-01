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

// Встраиваем бинарники прямо в ваш итоговый клиппер
//
//go:embed yt-dlp.exe
var ytdlpBytes []byte

//go:embed ffmpeg.exe
var ffmpegBytes []byte

//go:embed deno.exe
var denoBytes []byte

type LogEntry struct {
	ID    int    `json:"id"`
	Date  string `json:"date"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

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

// Парсинг метаданных и скачивание с поддержкой JS-рантайма и FFmpeg
func downloadAndGetTitle(ytdlpPath, ffmpegDir, denoPath, url string) (string, error) {

	// 1. Безопасный запрос метаданных (передаем рантайтм deno, чтобы не было ошибки "not available")
	cmdTitle := exec.Command(ytdlpPath,
		"--js-runtimes", "deno:"+denoPath,
		"--dump-json",
		url,
	)
	var outTitle bytes.Buffer
	cmdTitle.Stdout = &outTitle

	title := "Unknown YouTube Video"
	if err := cmdTitle.Run(); err == nil {
		var meta VideoMetadata
		if err := json.Unmarshal(outTitle.Bytes(), &meta); err == nil && meta.Title != "" {
			title = meta.Title
		}
	}

	// 2. Инициализация основного процесса загрузки
	// Добавили: --ffmpeg-location для склейки и --js-runtimes для обхода защиты
	cmd := exec.Command(ytdlpPath,
		"--js-runtimes", "deno:"+denoPath,
		"--ffmpeg-location", ffmpegDir,
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
	// Создаем временную папку в Temp
	tempDir := filepath.Join(os.TempDir(), "phonker_loader")
	_ = os.MkdirAll(tempDir, os.ModePerm)

	ytdlpPath := filepath.Join(tempDir, "yt-dlp.exe")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg.exe")
	denoPath := filepath.Join(tempDir, "deno.exe")

	// Распаковываем все три бинарника во временную директорию
	_ = os.WriteFile(ytdlpPath, ytdlpBytes, 0755)
	_ = os.WriteFile(ffmpegPath, ffmpegBytes, 0755)
	_ = os.WriteFile(denoPath, denoBytes, 0755)

	// Подчищаем за собой при выходе из программы
	defer os.Remove(ytdlpPath)
	defer os.Remove(ffmpegPath)
	defer os.Remove(denoPath)

	for {
		fmt.Println("\n=======================================")
		fmt.Println("     YOUTUBE DOWNLOADER BY PHONKER     ")
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

			// Передаем tempDir (где лежит ffmpeg) и путь к denoPath
			videoTitle, err := downloadAndGetTitle(ytdlpPath, tempDir, denoPath, url)

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
