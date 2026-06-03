package main

import (
	"bufio"
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

func downloadAndGetTitle(ytdlpPath, ffmpegDir, denoPath, url, quality, timeRange string) (string, error) {
	cmdTitle := exec.Command(ytdlpPath,
		"--js-runtimes", "deno:"+denoPath,
		"--dump-json",
		url,
	)
	var outTitle bytes.Buffer
	cmdTitle.Stdout = &outTitle

	title := "Unknown"
	if err := cmdTitle.Run(); err == nil {
		var meta VideoMetadata
		if err := json.Unmarshal(outTitle.Bytes(), &meta); err == nil && meta.Title != "" {
			title = meta.Title
		}
	}

	formatArg := "bv*[ext=mp4]+ba[ext=m4a]/bv*+ba/best"
	switch quality {
	case "1080":
		formatArg = "bv*[height<=1080][ext=mp4]+ba[ext=m4a]/bv*[height<=1080]+ba/best"
	case "720":
		formatArg = "bv*[height<=720][ext=mp4]+ba[ext=m4a]/bv*[height<=720]+ba/best"
	case "480":
		formatArg = "bv*[height<=480][ext=mp4]+ba[ext=m4a]/bv*[height<=480]+ba/best"
	case "360":
		formatArg = "bv*[height<=360][ext=mp4]+ba[ext=m4a]/bv*[height<=360]+ba/best"
	}

	args := []string{
		"--js-runtimes", "deno:" + denoPath,
		"--ffmpeg-location", ffmpegDir,
		"-f", formatArg,
		"--merge-output-format", "mp4",
		"--force-keyframes-at-cuts",
		"--downloader-args", "ffmpeg:-loglevel warning",
	}

	if timeRange != "" {
		args = append(args, "--download-sections", timeRange)
		args = append(args, "-o", "Downloads/%(title)s [Fragment].%(ext)s")
	} else {
		args = append(args, "-o", "Downloads/%(title)s.%(ext)s")
	}

	args = append(args, url)

	cmd := exec.Command(ytdlpPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	return title, err
}

func openDownloads() {
	downloadsPath := "Downloads"
	_ = os.MkdirAll(downloadsPath, os.ModePerm)
	_ = exec.Command("explorer", downloadsPath).Start()
}

func main() {
	tempDir := filepath.Join(os.TempDir(), "phonker_loader")
	_ = os.MkdirAll(tempDir, os.ModePerm)

	ytdlpPath := filepath.Join(tempDir, "yt-dlp.exe")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg.exe")
	denoPath := filepath.Join(tempDir, "deno.exe")

	_ = os.WriteFile(ytdlpPath, ytdlpBytes, 0755)
	_ = os.WriteFile(ffmpegPath, ffmpegBytes, 0755)
	_ = os.WriteFile(denoPath, denoBytes, 0755)

	defer os.Remove(ytdlpPath)
	defer os.Remove(ffmpegPath)
	defer os.Remove(denoPath)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n--- MENU ---")
		fmt.Println("[1] Скачать видео")
		fmt.Println("[2] Открыть папку")
		fmt.Println("[3] История")
		fmt.Println("[4] Выход")

		fmt.Print("\nВведите номер действия: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			fmt.Print("\nВведите URL: ")
			url, _ := reader.ReadString('\n')
			url = strings.TrimSpace(url)

			if url == "" {
				fmt.Println("Ошибка: пустой ввод.")
				continue
			}

			fmt.Println("\nКачество:")
			fmt.Println("[1] Максимальное")
			fmt.Println("[2] 1080p")
			fmt.Println("[3] 720p")
			fmt.Println("[4] 480p")
			fmt.Println("[5] 360p")
			fmt.Print("Введите номер: ")
			qChoice, _ := reader.ReadString('\n')
			qChoice = strings.TrimSpace(qChoice)

			quality := "best"
			switch qChoice {
			case "2":
				quality = "1080"
			case "3":
				quality = "720"
			case "4":
				quality = "480"
			case "5":
				quality = "360"
			}

			fmt.Println("\nРежим:")
			fmt.Println("[1] Полное видео")
			fmt.Println("[2] Фрагмент")
			fmt.Print("Введите номер: ")
			tChoice, _ := reader.ReadString('\n')
			tChoice = strings.TrimSpace(tChoice)

			timeRange := ""
			if tChoice == "2" {
				fmt.Println("\nФормат: ЧЧ:ММ:СС, ММ:СС или секунды")
				fmt.Print("Старт: ")
				start, _ := reader.ReadString('\n')
				start = strings.TrimSpace(start)

				fmt.Print("Конец: ")
				end, _ := reader.ReadString('\n')
				end = strings.TrimSpace(end)

				if start != "" && end != "" {
					timeRange = fmt.Sprintf("*%s-%s", start, end)
				} else {
					fmt.Println("Ошибка: тайм-коды не указаны. Загрузка полного видео.")
				}
			}

			fmt.Println("\nЗагрузка...")

			videoTitle, err := downloadAndGetTitle(ytdlpPath, tempDir, denoPath, url, quality, timeRange)

			if err != nil {
				fmt.Printf("\nОшибка: %v\n", err)
			} else {
				fmt.Println("\nГотово.")
				if timeRange != "" {
					videoTitle = videoTitle + " [Fragment]"
				}
				appendHistory(videoTitle, url)
				openDownloads()
			}

		case "2":
			openDownloads()

		case "3":
			fmt.Println("\n--- ИСТОРИЯ ---")
			history := readHistory()
			if len(history) == 0 {
				fmt.Println("История пуста.")
			} else {
				for i := len(history) - 1; i >= 0; i-- {
					fmt.Printf("ID %d | %s | %s\nURL: %s\n\n",
						history[i].ID, history[i].Date, history[i].Title, history[i].URL)
				}
			}

		case "4":
			return

		default:
			fmt.Println("Ошибка: неверный ввод.")
		}
	}
}
