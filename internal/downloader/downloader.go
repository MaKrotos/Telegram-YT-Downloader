package downloader

import (
	"YoutubeDownloader/internal/utils"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func DownloadYouTubeVideo(url string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	utils.CleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := utils.DiagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой: %v", err)
	}

	filename := filepath.Join(tmpDir, "ytvideo_"+utils.RandomString(8)+".mp4")
	absFilename, _ := filepath.Abs(filename)

	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	absYtDlpPath, _ := filepath.Abs(ytDlpPath)

	// Пробуем разные стратегии скачивания
	strategies := []struct {
		name string
		args []string
	}{
		{
			name: "best_quality",
			args: []string{"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "simple_best",
			args: []string{"-f", "best[ext=mp4]/best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "worst_quality",
			args: []string{"-f", "worst[ext=mp4]/worst", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
	}

	var lastError error
	for i, strategy := range strategies {
		fmt.Printf("[DOWNLOADER] Пробуем стратегию %d: %s\n", i+1, strategy.name)

		cmd := exec.Command(absYtDlpPath, strategy.args...)
		cmd.Args = append(cmd.Args, url)

		output, err := cmd.CombinedOutput()
		if err != nil {
			lastError = fmt.Errorf("yt-dlp error (strategy %s): %v, details: %s", strategy.name, err, string(output))
			fmt.Printf("[DOWNLOADER] Стратегия %s не удалась: %v\n", strategy.name, err)
			continue
		}

		// Проверяем, создался ли файл
		if _, err := os.Stat(absFilename); err == nil {
			fmt.Printf("[DOWNLOADER] Успешно скачано с помощью стратегии: %s\n", strategy.name)
			return absFilename, nil
		}

		// Ищем файл с другим расширением
		baseName := strings.TrimSuffix(absFilename, ".mp4")
		possibleExtensions := []string{".mp4", ".mkv", ".webm", ".avi", ".mov"}

		for _, ext := range possibleExtensions {
			altFilename := baseName + ext
			if _, err := os.Stat(altFilename); err == nil {
				fmt.Printf("[DOWNLOADER] Найден файл с расширением %s: %s\n", ext, altFilename)
				return altFilename, nil
			}
		}

		lastError = fmt.Errorf("файл не был создан после стратегии %s, yt-dlp output: %s", strategy.name, string(output))
	}

	return "", fmt.Errorf("все стратегии скачивания не удались. Последняя ошибка: %v", lastError)
}

// Функция для скачивания YouTube видео с уникальным идентификатором пользователя
func DownloadYouTubeVideoWithUserID(url string, userID int64, requestID string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	utils.CleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := utils.DiagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой: %v", err)
	}

	// Создаем уникальное имя файла с userID и requestID
	filename := filepath.Join(tmpDir, fmt.Sprintf("ytvideo_user%d_%s.mp4", userID, requestID))
	absFilename, _ := filepath.Abs(filename)

	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	absYtDlpPath, _ := filepath.Abs(ytDlpPath)

	// Пробуем разные стратегии скачивания
	strategies := []struct {
		name string
		args []string
	}{
		{
			name: "best_quality",
			args: []string{"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "simple_best",
			args: []string{"-f", "best[ext=mp4]/best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "worst_quality",
			args: []string{"-f", "worst[ext=mp4]/worst", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
	}

	var lastError error
	for i, strategy := range strategies {
		fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: пробуем стратегию %d: %s\n", requestID, userID, i+1, strategy.name)

		cmd := exec.Command(absYtDlpPath, strategy.args...)
		cmd.Args = append(cmd.Args, url)

		output, err := cmd.CombinedOutput()
		if err != nil {
			lastError = fmt.Errorf("yt-dlp error (strategy %s): %v, details: %s", strategy.name, err, string(output))
			fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: стратегия %s не удалась: %v\n", requestID, userID, strategy.name, err)
			continue
		}

		// Проверяем, создался ли файл
		if _, err := os.Stat(absFilename); err == nil {
			fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: успешно скачано с помощью стратегии: %s\n", requestID, userID, strategy.name)
			return absFilename, nil
		}

		// Ищем файл с другим расширением
		baseName := strings.TrimSuffix(absFilename, ".mp4")
		possibleExtensions := []string{".mp4", ".mkv", ".webm", ".avi", ".mov"}

		for _, ext := range possibleExtensions {
			altFilename := baseName + ext
			if _, err := os.Stat(altFilename); err == nil {
				fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: найден файл с расширением %s: %s\n", requestID, userID, ext, altFilename)
				return altFilename, nil
			}
		}

		lastError = fmt.Errorf("файл не был создан после стратегии %s, yt-dlp output: %s", strategy.name, string(output))
	}

	return "", fmt.Errorf("все стратегии скачивания не удались для пользователя %d (request %s). Последняя ошибка: %v", userID, requestID, lastError)
}

// Функция для скачивания YouTube видео с уникальным идентификатором пользователя и URL хешем
func DownloadYouTubeVideoWithUserIDAndURL(url string, userID int64, requestID string, urlHash string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	utils.CleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := utils.DiagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой: %v", err)
	}

	// Создаем уникальное имя файла с userID, requestID и urlHash
	filename := filepath.Join(tmpDir, fmt.Sprintf("ytvideo_user%d_%s_%s.mp4", userID, requestID, urlHash))
	absFilename, _ := filepath.Abs(filename)

	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	absYtDlpPath, _ := filepath.Abs(ytDlpPath)

	// Пробуем разные стратегии скачивания
	strategies := []struct {
		name string
		args []string
	}{
		{
			name: "best_quality",
			args: []string{"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "simple_best",
			args: []string{"-f", "best[ext=mp4]/best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "worst_quality",
			args: []string{"-f", "worst[ext=mp4]/worst", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
	}

	var lastError error
	for i, strategy := range strategies {
		fmt.Printf("[DOWNLOADER] [%s] Пользователь %d (URL: %s): пробуем стратегию %d: %s\n", requestID, userID, urlHash, i+1, strategy.name)

		cmd := exec.Command(absYtDlpPath, strategy.args...)
		cmd.Args = append(cmd.Args, url)

		output, err := cmd.CombinedOutput()
		if err != nil {
			lastError = fmt.Errorf("yt-dlp error (strategy %s): %v, details: %s", strategy.name, err, string(output))
			fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: стратегия %s не удалась: %v\n", requestID, userID, strategy.name, err)
			continue
		}

		// Проверяем, создался ли файл
		if _, err := os.Stat(absFilename); err == nil {
			fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: успешно скачано с помощью стратегии: %s\n", requestID, userID, strategy.name)
			return absFilename, nil
		}

		// Ищем файл с другим расширением
		baseName := strings.TrimSuffix(absFilename, ".mp4")
		possibleExtensions := []string{".mp4", ".mkv", ".webm", ".avi", ".mov"}

		for _, ext := range possibleExtensions {
			altFilename := baseName + ext
			if _, err := os.Stat(altFilename); err == nil {
				fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: найден файл с расширением %s: %s\n", requestID, userID, ext, altFilename)
				return altFilename, nil
			}
		}

		lastError = fmt.Errorf("файл не был создан после стратегии %s, yt-dlp output: %s", strategy.name, string(output))
	}

	return "", fmt.Errorf("все стратегии скачивания не удались для пользователя %d (request %s, URL %s). Последняя ошибка: %v", userID, requestID, urlHash, lastError)
}
