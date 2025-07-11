package downloader

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func DownloadYouTubeVideo(url string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	cleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := diagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой: %v", err)
	}

	filename := filepath.Join(tmpDir, "ytvideo_"+randomString(8)+".mp4")
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

func DownloadTikTokVideo(url string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	cleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := diagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой для TikTok: %v", err)
	}

	filename := filepath.Join(tmpDir, "tiktok_"+randomString(8)+".mp4")
	absFilename, _ := filepath.Abs(filename)

	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	absYtDlpPath, _ := filepath.Abs(ytDlpPath)

	// Пробуем разные стратегии для TikTok
	strategies := []struct {
		name string
		args []string
	}{
		{
			name: "tiktok_best",
			args: []string{"-f", "mp4", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "tiktok_any",
			args: []string{"-f", "best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "tiktok_worst",
			args: []string{"-f", "worst", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
	}

	var lastError error
	for i, strategy := range strategies {
		fmt.Printf("[DOWNLOADER] Пробуем TikTok стратегию %d: %s\n", i+1, strategy.name)

		cmd := exec.Command(absYtDlpPath, strategy.args...)
		cmd.Args = append(cmd.Args, url)

		output, err := cmd.CombinedOutput()
		if err != nil {
			lastError = fmt.Errorf("yt-dlp TikTok error (strategy %s): %v, details: %s", strategy.name, err, string(output))
			fmt.Printf("[DOWNLOADER] TikTok стратегия %s не удалась: %v\n", strategy.name, err)
			continue
		}

		// Проверяем, создался ли файл
		if _, err := os.Stat(absFilename); err == nil {
			fmt.Printf("[DOWNLOADER] TikTok успешно скачан с помощью стратегии: %s\n", strategy.name)
			return absFilename, nil
		}

		// Ищем файл с другим расширением
		baseName := strings.TrimSuffix(absFilename, ".mp4")
		possibleExtensions := []string{".mp4", ".mkv", ".webm", ".avi", ".mov"}

		for _, ext := range possibleExtensions {
			altFilename := baseName + ext
			if _, err := os.Stat(altFilename); err == nil {
				fmt.Printf("[DOWNLOADER] Найден TikTok файл с расширением %s: %s\n", ext, altFilename)
				return altFilename, nil
			}
		}

		lastError = fmt.Errorf("TikTok файл не был создан после стратегии %s, yt-dlp output: %s", strategy.name, string(output))
	}

	return "", fmt.Errorf("все TikTok стратегии скачивания не удались. Последняя ошибка: %v", lastError)
}

// Функция для скачивания YouTube видео с уникальным идентификатором пользователя
func DownloadYouTubeVideoWithUserID(url string, userID int64, requestID string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	cleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := diagnoseFileSystem(tmpDir); err != nil {
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

// Функция для скачивания TikTok видео с уникальным идентификатором пользователя
func DownloadTikTokVideoWithUserID(url string, userID int64, requestID string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	cleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := diagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой для TikTok: %v", err)
	}

	// Создаем уникальное имя файла с userID и requestID
	filename := filepath.Join(tmpDir, fmt.Sprintf("tiktok_user%d_%s.mp4", userID, requestID))
	absFilename, _ := filepath.Abs(filename)

	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	absYtDlpPath, _ := filepath.Abs(ytDlpPath)

	// Пробуем разные стратегии для TikTok
	strategies := []struct {
		name string
		args []string
	}{
		{
			name: "tiktok_best",
			args: []string{"-f", "mp4", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "tiktok_any",
			args: []string{"-f", "best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "tiktok_worst",
			args: []string{"-f", "worst", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
	}

	var lastError error
	for i, strategy := range strategies {
		fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: пробуем TikTok стратегию %d: %s\n", requestID, userID, i+1, strategy.name)

		cmd := exec.Command(absYtDlpPath, strategy.args...)
		cmd.Args = append(cmd.Args, url)

		output, err := cmd.CombinedOutput()
		if err != nil {
			lastError = fmt.Errorf("yt-dlp TikTok error (strategy %s): %v, details: %s", strategy.name, err, string(output))
			fmt.Printf("[DOWNLOADER] Пользователь %d: TikTok стратегия %s не удалась: %v\n", userID, strategy.name, err)
			continue
		}

		// Проверяем, создался ли файл
		if _, err := os.Stat(absFilename); err == nil {
			fmt.Printf("[DOWNLOADER] Пользователь %d: TikTok успешно скачан с помощью стратегии: %s\n", userID, strategy.name)
			return absFilename, nil
		}

		// Ищем файл с другим расширением
		baseName := strings.TrimSuffix(absFilename, ".mp4")
		possibleExtensions := []string{".mp4", ".mkv", ".webm", ".avi", ".mov"}

		for _, ext := range possibleExtensions {
			altFilename := baseName + ext
			if _, err := os.Stat(altFilename); err == nil {
				fmt.Printf("[DOWNLOADER] Пользователь %d: найден TikTok файл с расширением %s: %s\n", userID, ext, altFilename)
				return altFilename, nil
			}
		}

		lastError = fmt.Errorf("TikTok файл не был создан после стратегии %s, yt-dlp output: %s", strategy.name, string(output))
	}

	return "", fmt.Errorf("все TikTok стратегии скачивания не удались для пользователя %d. Последняя ошибка: %v", userID, lastError)
}

// Функция для скачивания YouTube видео с уникальным идентификатором пользователя и URL хешем
func DownloadYouTubeVideoWithUserIDAndURL(url string, userID int64, requestID string, urlHash string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	cleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := diagnoseFileSystem(tmpDir); err != nil {
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

// Функция для скачивания TikTok видео с уникальным идентификатором пользователя и URL хешем
func DownloadTikTokVideoWithUserIDAndURL(url string, userID int64, requestID string, urlHash string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
	}

	// Очищаем старые временные файлы
	cleanupTempFiles(tmpDir)

	// Диагностируем файловую систему
	if err := diagnoseFileSystem(tmpDir); err != nil {
		return "", fmt.Errorf("проблемы с файловой системой для TikTok: %v", err)
	}

	// Создаем уникальное имя файла с userID, requestID и urlHash
	filename := filepath.Join(tmpDir, fmt.Sprintf("tiktok_user%d_%s_%s.mp4", userID, requestID, urlHash))
	absFilename, _ := filepath.Abs(filename)

	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	absYtDlpPath, _ := filepath.Abs(ytDlpPath)

	// Пробуем разные стратегии для TikTok
	strategies := []struct {
		name string
		args []string
	}{
		{
			name: "tiktok_best",
			args: []string{"-f", "mp4", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "tiktok_any",
			args: []string{"-f", "best", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
		{
			name: "tiktok_worst",
			args: []string{"-f", "worst", "-o", absFilename, "--no-mtime", "--no-warnings"},
		},
	}

	var lastError error
	for i, strategy := range strategies {
		fmt.Printf("[DOWNLOADER] [%s] Пользователь %d (URL: %s): пробуем TikTok стратегию %d: %s\n", requestID, userID, urlHash, i+1, strategy.name)

		cmd := exec.Command(absYtDlpPath, strategy.args...)
		cmd.Args = append(cmd.Args, url)

		output, err := cmd.CombinedOutput()
		if err != nil {
			lastError = fmt.Errorf("yt-dlp TikTok error (strategy %s): %v, details: %s", strategy.name, err, string(output))
			fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: TikTok стратегия %s не удалась: %v\n", requestID, userID, strategy.name, err)
			continue
		}

		// Проверяем, создался ли файл
		if _, err := os.Stat(absFilename); err == nil {
			fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: TikTok успешно скачан с помощью стратегии: %s\n", requestID, userID, strategy.name)
			return absFilename, nil
		}

		// Ищем файл с другим расширением
		baseName := strings.TrimSuffix(absFilename, ".mp4")
		possibleExtensions := []string{".mp4", ".mkv", ".webm", ".avi", ".mov"}

		for _, ext := range possibleExtensions {
			altFilename := baseName + ext
			if _, err := os.Stat(altFilename); err == nil {
				fmt.Printf("[DOWNLOADER] [%s] Пользователь %d: найден TikTok файл с расширением %s: %s\n", requestID, userID, ext, altFilename)
				return altFilename, nil
			}
		}

		lastError = fmt.Errorf("TikTok файл не был создан после стратегии %s, yt-dlp output: %s", strategy.name, string(output))
	}

	return "", fmt.Errorf("все TikTok стратегии скачивания не удались для пользователя %d (request %s, URL %s). Последняя ошибка: %v", userID, requestID, urlHash, lastError)
}

// Функция для очистки старых временных файлов
func cleanupTempFiles(tmpDir string) {
	// Удаляем файлы старше 1 часа
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-1 * time.Hour)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Удаляем только файлы, которые мы создаем (с префиксами ytvideo_, tiktok_)
		fileName := entry.Name()
		if (strings.HasPrefix(fileName, "ytvideo_") || strings.HasPrefix(fileName, "tiktok_")) && info.ModTime().Before(cutoff) {
			filePath := filepath.Join(tmpDir, fileName)
			os.Remove(filePath)
			fmt.Printf("[DOWNLOADER] Удален старый временный файл: %s\n", filePath)
		}
	}
}

// Функция для диагностики проблем с файловой системой
func diagnoseFileSystem(tmpDir string) error {
	// Проверяем, существует ли папка
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		return fmt.Errorf("папка %s не существует", tmpDir)
	}

	// Проверяем права доступа
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать/проверить папку %s: %v", tmpDir, err)
	}

	// Пробуем создать тестовый файл
	testFile := filepath.Join(tmpDir, "test_"+randomString(4)+".tmp")
	testContent := []byte("test")

	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		return fmt.Errorf("не удалось создать тестовый файл в %s: %v", tmpDir, err)
	}

	// Проверяем, что файл создался
	if _, err := os.Stat(testFile); err != nil {
		return fmt.Errorf("тестовый файл не найден после создания: %v", err)
	}

	// Удаляем тестовый файл
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("не удалось удалить тестовый файл: %v", err)
	}

	// Проверяем свободное место (упрощенная версия)
	// Пробуем создать файл размером 1 МБ для проверки места
	testLargeFile := filepath.Join(tmpDir, "test_large_"+randomString(4)+".tmp")
	testLargeContent := make([]byte, 1024*1024) // 1 МБ

	if err := os.WriteFile(testLargeFile, testLargeContent, 0644); err != nil {
		return fmt.Errorf("недостаточно места на диске или проблемы с правами доступа: %v", err)
	}

	// Удаляем тестовый файл
	if err := os.Remove(testLargeFile); err != nil {
		return fmt.Errorf("не удалось удалить тестовый файл: %v", err)
	}

	fmt.Printf("[DOWNLOADER] Диагностика файловой системы: OK (достаточно места для скачивания)\n")
	return nil
}

func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[int64(i+os.Getpid()+n)%int64(len(letters))]
	}
	return string(b)
}
