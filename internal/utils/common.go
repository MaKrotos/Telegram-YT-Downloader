package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Генерация случайной строки
func RandomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[int64(i+os.Getpid()+n)%int64(len(letters))]
	}
	return string(b)
}

// Очистка старых временных файлов
func CleanupTempFiles(tmpDir string) {
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
		fileName := entry.Name()
		if strings.HasPrefix(fileName, "ytvideo_") && info.ModTime().Before(cutoff) {
			filePath := filepath.Join(tmpDir, fileName)
			os.Remove(filePath)
			fmt.Printf("[DOWNLOADER] Удален старый временный файл: %s\n", filePath)
		}
	}
}

// Диагностика проблем с файловой системой
func DiagnoseFileSystem(tmpDir string) error {
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		return fmt.Errorf("папка %s не существует", tmpDir)
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("не удалось создать/проверить папку %s: %v", tmpDir, err)
	}
	testFile := filepath.Join(tmpDir, "test_"+RandomString(4)+".tmp")
	testContent := []byte("test")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		return fmt.Errorf("не удалось создать тестовый файл в %s: %v", tmpDir, err)
	}
	if _, err := os.Stat(testFile); err != nil {
		return fmt.Errorf("тестовый файл не найден после создания: %v", err)
	}
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("не удалось удалить тестовый файл: %v", err)
	}
	testLargeFile := filepath.Join(tmpDir, "test_large_"+RandomString(4)+".tmp")
	testLargeContent := make([]byte, 1024*1024)
	if err := os.WriteFile(testLargeFile, testLargeContent, 0644); err != nil {
		return fmt.Errorf("недостаточно места на диске или проблемы с правами доступа: %v", err)
	}
	if err := os.Remove(testLargeFile); err != nil {
		return fmt.Errorf("не удалось удалить тестовый файл: %v", err)
	}
	fmt.Printf("[DOWNLOADER] Диагностика файловой системы: OK (достаточно места для скачивания)\n")
	return nil
}
