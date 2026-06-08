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
	"sync"
	"time"
)

//go:embed yt-dlp.exe
var ytdlpBytes []byte

//go:embed ffmpeg.exe
var ffmpegBytes []byte

//go:embed ffprobe.exe
var ffprobeBytes []byte

//go:embed deno.exe
var denoBytes []byte

type LogEntry struct {
	ID    int    `json:"id"`
	Date  string `json:"date"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type VideoMetadata struct {
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
}

// Глобальная переменная для хранения истории в оперативной памяти
var memoryHistory []LogEntry

func appendHistory(title, url string) {
	newID := 1
	if len(memoryHistory) > 0 {
		newID = memoryHistory[len(memoryHistory)-1].ID + 1
	}

	newEntry := LogEntry{
		ID:    newID,
		Date:  time.Now().Format("2006-01-02 15:04:05"),
		Title: title,
		URL:   url,
	}

	memoryHistory = append(memoryHistory, newEntry)
}

func startSpinner(message string, stopChan chan bool, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		frames := []string{"|", "/", "-", "\\"}
		i := 0
		for {
			select {
			case <-stopChan:
				fmt.Print("\r\033[K")
				return
			default:
				fmt.Printf("\r[%s] %s", frames[i], message)
				i = (i + 1) % len(frames)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

func formatSeconds(seconds float64) string {
	if seconds <= 0 {
		return "00:00"
	}
	d := time.Duration(seconds) * time.Second
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func downloadAndGetTitle(ytdlpPath, ffmpegDir, denoPath, url, quality, timeRange string) (string, string, error) {
	title := "Unknown"
	var duration float64 = 0
	detectedBrowser := ""
	useCookieFile := false

	if _, err := os.Stat("cookies.txt"); err == nil {
		useCookieFile = true
	}

	isYoutube := strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")

	if timeRange != "GET_INFO_ONLY" {
		goto skipMetadata
	}

	{
		stopSpinnerChan := make(chan bool)
		var spinnerWg sync.WaitGroup
		startSpinner("Получение метаданных видео и обход защиты...", stopSpinnerChan, &spinnerWg)

		argsTitle := []string{"--dump-json", url}
		if isYoutube {
			argsTitle = append(argsTitle, "--extractor-args", "youtube:player-client=android,web_embedded")
		}

		if useCookieFile {
			argsTitle = append(argsTitle, "--cookies", "cookies.txt")
		} else {
			browsers := []string{"brave", "chrome", "opera", "firefox", "edge", "vivaldi"}
			for _, br := range browsers {
				testArgs := []string{"--cookies-from-browser", br, "--dump-json", url}
				if isYoutube {
					testArgs = append([]string{
						"--js-runtimes", "deno:" + denoPath,
						"--extractor-args", "youtube:player-client=android,web_embedded",
					}, testArgs...)
				}

				cmdTest := exec.Command(ytdlpPath, testArgs...)
				var outTest bytes.Buffer
				cmdTest.Stdout = &outTest
				cmdTest.Stderr = nil

				if err := cmdTest.Run(); err == nil {
					var meta VideoMetadata
					if err := json.Unmarshal(outTest.Bytes(), &meta); err == nil && meta.Title != "" {
						title = meta.Title
						duration = meta.Duration
						detectedBrowser = br
						break
					}
				}
			}
		}

		if title == "Unknown" {
			cmdTitleArgs := argsTitle
			if isYoutube {
				cmdTitleArgs = append([]string{"--js-runtimes", "deno:" + denoPath}, cmdTitleArgs...)
			}
			cmdTitle := exec.Command(ytdlpPath, cmdTitleArgs...)
			var outTitle bytes.Buffer
			cmdTitle.Stdout = &outTitle
			cmdTitle.Stderr = nil

			if err := cmdTitle.Run(); err == nil {
				var meta VideoMetadata
				if err := json.Unmarshal(outTitle.Bytes(), &meta); err == nil && meta.Title != "" {
					title = meta.Title
					duration = meta.Duration
				}
			}
		}

		stopSpinnerChan <- true
		spinnerWg.Wait()

		if title == "Unknown" && isYoutube {
			title = "YouTube Video [Age Restricted]"
		}

		if detectedBrowser != "" {
			fmt.Printf("\nАвторизация успешна через профиль браузера: %s\n", detectedBrowser)
		} else {
			fmt.Println()
		}
		fmt.Printf("Название видео: \"%s\"\n", title)

		return title, formatSeconds(duration), nil
	}

skipMetadata:

	formatArg := "bv*[ext=mp4]+ba[ext=m4a]/bv*+ba/best"
	if strings.Contains(url, "twitch.tv") {
		switch quality {
		case "1080":
			formatArg = "1080p/1080p60/best"
		case "720":
			formatArg = "720p/720p60/720p30"
		case "480":
			formatArg = "480p/480p30"
		case "360":
			formatArg = "360p/360p30"
		default:
			formatArg = "best/source"
		}
	} else {
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
	}

	args := []string{
		"--ffmpeg-location", ffmpegDir,
		"-f", formatArg,
		"--merge-output-format", "mp4",
		"--force-ipv4",
		"--no-warnings",
		"--socket-timeout", "60",

		"--quiet",
		"--progress",
		"--progress-template", "\r[download] %(progress._percent_str)s | Вес: %(progress._total_bytes_str)s | Скорость: %(progress._speed_str)s | Прошло времени: %(progress._elapsed_str)s",
	}

	if isYoutube {
		args = append(args, "--extractor-args", "youtube:player-client=android,web_embedded")
	}

	isFragment := timeRange != ""
	_ = os.MkdirAll("Downloads", os.ModePerm)

	if isFragment {
		fmt.Println("Инициализация нарезки фрагмента...")
		fmt.Println("Внимание: на длинных видео спиннер будет крутиться до завершения кэширования.")

		args = append(args, "--download-sections", timeRange)
		args = append(args, "-o", "Downloads/%(title)s [Fragment].%(ext)s")
		args = append(args, "--downloader-args", "ffmpeg_i:-seekable 0")
		args = append(args, "--downloader-args", "ffmpeg:-loglevel warning")
		args = append(args, "--concurrent-fragments", "1")
	} else {
		fmt.Println("Скачивание полного видео...")
		args = append(args, "-o", "Downloads/%(title)s.%(ext)s")
		args = append(args, "--downloader-args", "ffmpeg:-loglevel warning")
		args = append(args, "--concurrent-fragments", "4")
	}

	if useCookieFile {
		args = append(args, "--cookies", "cookies.txt")
	} else {
		args = append(args, "--cookies-from-browser", "chrome,brave,firefox,edge,opera,vivaldi")
	}

	if isYoutube {
		args = append([]string{"--js-runtimes", "deno:" + denoPath}, args...)
	}

	args = append(args, url)
	cmd := exec.Command(ytdlpPath, args...)

	if isFragment {
		stopDownloadSpinner := make(chan bool)
		var downloadSpinnerWg sync.WaitGroup
		startSpinner("Обработка и загрузка фрагмента потока...", stopDownloadSpinner, &downloadSpinnerWg)

		cmd.Stdout = nil
		cmd.Stderr = nil
		err := cmd.Run()

		stopDownloadSpinner <- true
		downloadSpinnerWg.Wait()
		return "", "", err
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()

		if err == nil {
			fmt.Println()
		}
		return "", "", err
	}
}

func openDownloads() {
	downloadsPath := "Downloads"
	_ = os.MkdirAll(downloadsPath, os.ModePerm)
	_ = exec.Command("explorer", downloadsPath).Start()
}

func writeEmbeddedFile(path string, data []byte) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, data, 0755)
	}
}

func main() {
	tempDir := filepath.Join(os.TempDir(), "phonker_loader")
	_ = os.MkdirAll(tempDir, os.ModePerm)

	ytdlpPath := filepath.Join(tempDir, "yt-dlp.exe")
	ffmpegPath := filepath.Join(tempDir, "ffmpeg.exe")
	ffprobePath := filepath.Join(tempDir, "ffprobe.exe")
	denoPath := filepath.Join(tempDir, "deno.exe")

	writeEmbeddedFile(ytdlpPath, ytdlpBytes)
	writeEmbeddedFile(ffmpegPath, ffmpegBytes)
	writeEmbeddedFile(ffprobePath, ffprobeBytes)
	writeEmbeddedFile(denoPath, denoBytes)

	defer os.RemoveAll(tempDir)

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("\n=======================")
		fmt.Println("      PHONKER LOADER     ")
		fmt.Println("=======================")
		fmt.Println("[1] Скачать контент (YouTube / Twitch)")
		fmt.Println("[2] Открыть папку загрузок")
		fmt.Println("[3] Посмотреть историю (текущая сессия)")
		fmt.Println("[4] Выйти")

		fmt.Print("\nВведи номер действия: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			fmt.Print("\nВставь ссылку на видео: ")
			url, _ := reader.ReadString('\n')
			url = strings.TrimSpace(url)

			if url == "" {
				fmt.Println("Ошибка: пустая ссылка.")
				continue
			}

			fmt.Println("\nВыбери качество:")
			fmt.Println(" [1] Максимальное")
			fmt.Println(" [2] 1080p")
			fmt.Println(" [3] 720p")
			fmt.Println(" [4] 480p")
			fmt.Println(" [5] 360p")
			fmt.Print("Введи номер: ")
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

			title, maxDuration, err := downloadAndGetTitle(ytdlpPath, tempDir, denoPath, url, quality, "GET_INFO_ONLY")
			if err != nil {
				fmt.Printf("\nОшибка при получении метаданных: %v\n", err)
				continue
			}

			startZero := "00:00"
			if len(maxDuration) > 5 {
				startZero = "00:00:00"
			}

			fmt.Printf("\nНужно вырезать фрагмент? (Доступно: %s-%s)\n", startZero, maxDuration)
			fmt.Print("Введите в формате *00:01:30-00:02:15 или нажмите Enter для полной загрузки: ")
			timeRange, _ := reader.ReadString('\n')
			timeRange = strings.TrimSpace(timeRange)

			_, _, err = downloadAndGetTitle(ytdlpPath, tempDir, denoPath, url, quality, timeRange)
			if err != nil {
				fmt.Printf("\nОшибка при загрузке: %v\n", err)
			} else {
				fmt.Println("\nЗагрузка успешно завершена!")
				appendHistory(title, url) // Запись идет в ОЗУ
			}

		case "2":
			openDownloads()
			fmt.Println("\nПапка загрузок открыта.")

		case "3":
			// Чтение происходит напрямую из глобального слайса в ОЗУ
			if len(memoryHistory) == 0 {
				fmt.Println("\nИстория текущей сессии пуста.")
			} else {
				fmt.Println("\n=== ИСТОРИЯ ЗАГРУЗОК (В ПАМЯТИ) ===")
				for _, entry := range memoryHistory {
					fmt.Printf("[%d] %s | %s\n    Ссылка: %s\n", entry.ID, entry.Date, entry.Title, entry.URL)
				}
			}

		case "4":
			fmt.Println("Выход из программы...")
			return

		default:
			fmt.Println("Неверный ввод. Попробуйте еще раз.")
		}
	}
}
