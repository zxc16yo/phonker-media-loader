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
	Time  string `json:"time"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type VideoMetadata struct {
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
}

const historyFilePath = "Downloads/history.json"

func readHistoryFromFile() []LogEntry {
	_ = os.MkdirAll("Downloads", os.ModePerm)
	var list []LogEntry
	data, err := os.ReadFile(historyFilePath)
	if err != nil {
		return list
	}
	_ = json.Unmarshal(data, &list)
	return list
}

func appendHistoryDirectly(title, url string) {
	_ = os.MkdirAll("Downloads", os.ModePerm)
	currentEntries := readHistoryFromFile()
	newID := 1
	if len(currentEntries) > 0 {
		newID = currentEntries[len(currentEntries)-1].ID + 1
	}
	newEntry := LogEntry{
		ID:    newID,
		Date:  time.Now().Format("2006-01-02"),
		Time:  time.Now().Format("15:04:05"),
		Title: title,
		URL:   url,
	}
	currentEntries = append(currentEntries, newEntry)
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(currentEntries); err == nil {
		_ = os.WriteFile(historyFilePath, buf.Bytes(), 0644)
	}
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

func normalizeTime(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return ""
	}
	if t == "0" || t == "00" || t == "0:00" || t == "00:00" || t == "00:00:00" {
		return "00:00:00"
	}
	parts := strings.Split(t, ":")
	if len(parts) == 1 {
		var s int
		fmt.Sscanf(parts[0], "%d", &s)
		return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
	}
	if len(parts) == 2 {
		return fmt.Sprintf("00:%02s:%02s", parts[0], parts[1])
	}
	if len(parts) == 3 {
		return fmt.Sprintf("%02s:%02s:%02s", parts[0], parts[1], parts[2])
	}
	return t
}

func downloadVideo(ytdlpPath, ffmpegDir, denoPath, url, quality, startTime, endTime string) (string, error) {
	useCookieFile := false
	if _, err := os.Stat("cookies.txt"); err == nil {
		useCookieFile = true
	}

	isYoutube := strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be")

	stopSpinnerChan := make(chan bool)
	var spinnerWg sync.WaitGroup
	startSpinner("Проверка ссылки и получение метаданных видео...", stopSpinnerChan, &spinnerWg)

	argsTitle := []string{}
	if isYoutube {
		argsTitle = append(argsTitle, "--js-runtimes", "deno:"+denoPath)
	}
	argsTitle = append(argsTitle, "--dump-json")
	if isYoutube {
		argsTitle = append(argsTitle, "--extractor-args", "youtube:player-client=android,web_embedded")
	}
	if useCookieFile {
		argsTitle = append(argsTitle, "--cookies", "cookies.txt")
	} else {
		argsTitle = append(argsTitle, "--cookies-from-browser", "chrome,brave,firefox,edge,opera,vivaldi")
	}
	argsTitle = append(argsTitle, url)

	cmdTitle := exec.Command(ytdlpPath, argsTitle...)
	var outTitle bytes.Buffer
	cmdTitle.Stdout = &outTitle
	if err := cmdTitle.Run(); err != nil {
		stopSpinnerChan <- true
		spinnerWg.Wait()
		return "", fmt.Errorf("видео недоступно или ссылка неверная")
	}

	var meta VideoMetadata
	_ = json.Unmarshal(outTitle.Bytes(), &meta)
	stopSpinnerChan <- true
	spinnerWg.Wait()

	if meta.Title == "" {
		meta.Title = "Downloaded_Video"
	}
	fmt.Printf("\nНазвание видео: \"%s\" [%s]\n", meta.Title, formatSeconds(meta.Duration))

	safeTitle := meta.Title
	for _, char := range []string{"<", ">", ":", "\"", "/", "\\", "|", "?", "*"} {
		safeTitle = strings.ReplaceAll(safeTitle, char, "")
	}

	formatArg := "bv*[ext=mp4]+ba[ext=m4a]/bv*+ba/best"
	if !strings.Contains(url, "twitch.tv") {
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

	args := []string{}
	if isYoutube {
		args = append(args, "--js-runtimes", "deno:"+denoPath)
	}

	args = append(args,
		"--ffmpeg-location", ffmpegDir,
		"-f", formatArg,
		"--force-ipv4",
		"--no-warnings",
		"--quiet",
		"--merge-output-format", "mp4",
	)

	if isYoutube {
		args = append(args, "--extractor-args", "youtube:player-client=android,web_embedded")
	}
	if useCookieFile {
		args = append(args, "--cookies", "cookies.txt")
	} else {
		args = append(args, "--cookies-from-browser", "chrome,brave,firefox,edge,opera,vivaldi")
	}

	_ = os.MkdirAll("Downloads", os.ModePerm)

	stopDownloadSpinner := make(chan bool)
	var downloadSpinnerWg sync.WaitGroup

	// Если startTime заполнена (не пустая строка) — это ВСЕГДА нарезка фрагмента
	if startTime != "" {
		startSpinner("Выполняется загрузка и нарезка фрагмента потока...", stopDownloadSpinner, &downloadSpinnerWg)
		args = append(args, "--downloader", "ffmpeg")
		args = append(args, "--downloader-args", fmt.Sprintf("ffmpeg:-ss %s -to %s", startTime, endTime))
		args = append(args, "-o", filepath.Join("Downloads", safeTitle+" [Fragment].%(ext)s"))
	} else {
		// Оставили пустым -> качаем всё
		startSpinner("Выполняется скачивание полного видео...", stopDownloadSpinner, &downloadSpinnerWg)
		args = append(args, "--concurrent-fragments", "4")
		args = append(args, "-o", filepath.Join("Downloads", safeTitle+".%(ext)s"))
	}

	args = append(args, url)

	cmd := exec.Command(ytdlpPath, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()

	stopDownloadSpinner <- true
	downloadSpinnerWg.Wait()

	if err != nil {
		return meta.Title, fmt.Errorf("ошибка во время загрузки: %v", err)
	}

	return meta.Title, nil
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
		fmt.Println("\n=============================================")
		fmt.Println("               PHONKER LOADER                ")
		fmt.Println("=============================================")
		fmt.Println("[1] Скачать контент (YouTube / Twitch)")
		fmt.Println("[2] Открыть папку загрузок")
		fmt.Println("[3] Посмотреть историю загрузок")
		fmt.Println("[4] Выйти")
		fmt.Println("=============================================")

		fmt.Print("\nВведи номер действия: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			fmt.Print("\nВставь ссылку на видео: ")
			url, _ := reader.ReadString('\n')
			url = strings.TrimSpace(url)
			if url == "" {
				continue
			}

			fmt.Println("\nВыбери качество видео:")
			fmt.Println(" [1] Максимальное\n [2] 1080p\n [3] 720p\n [4] 480p\n [5] 360p")
			fmt.Print("Введи номер: ")
			qChoice, _ := reader.ReadString('\n')
			quality := "best"
			switch strings.TrimSpace(qChoice) {
			case "2":
				quality = "1080"
			case "3":
				quality = "720"
			case "4":
				quality = "480"
			case "5":
				quality = "360"
			}

			fmt.Println("\n--- НАСТРОЙКА ТАЙМКОДОВ ---")
			fmt.Print("Время НАЧАЛА (например: 0, 30, 1:15 или Enter чтобы скачать ВСЁ видео): ")
			startIn, _ := reader.ReadString('\n')
			startIn = strings.TrimSpace(startIn)

			var startTime, endTime string
			if startIn != "" {
				startTime = normalizeTime(startIn)
				fmt.Print("Время ОКОНЧАНИЯ (например: 65 или 2:40): ")
				endIn, _ := reader.ReadString('\n')
				endTime = normalizeTime(endIn)
			} else {
				startTime = "" // Флаг для скачивания полного видео
			}

			title, err := downloadVideo(ytdlpPath, tempDir, denoPath, url, quality, startTime, endTime)
			if err != nil {
				fmt.Printf("\n[Ошибка] %v\n", err)
			} else {
				fmt.Println("\rЗагрузка успешно завершена!")
				appendHistoryDirectly(title, url)
			}

		case "2":
			_ = exec.Command("explorer", "Downloads").Start()
		case "3":
			history := readHistoryFromFile()
			if len(history) == 0 {
				fmt.Println("\nИстория пуста.")
			} else {
				fmt.Println("\n=============================================")
				fmt.Println("         ИСТОРИЯ ЗАГРУЗОК ИЗ JSON            ")
				fmt.Println("=============================================")
				for _, entry := range history {
					fmt.Printf("[%d] %s в %s | %s\n Ссылка: %s\n", entry.ID, entry.Date, entry.Time, entry.Title, entry.URL)
				}
				fmt.Println("=============================================")
			}
		case "4":
			return
		}
	}
}

func writeEmbeddedFile(path string, data []byte) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, data, 0755)
	}
}
