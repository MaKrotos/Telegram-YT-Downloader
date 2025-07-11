package downloader

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func DownloadYouTubeVideo(url string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
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
	cmd := exec.Command(absYtDlpPath, "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best", "-o", absFilename, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New("yt-dlp error: " + err.Error() + ", details: " + string(output))
	}
	if _, err := os.Stat(absFilename); err != nil {
		return "", errors.New("файл не был создан: " + err.Error() + ", yt-dlp output: " + string(output))
	}
	return absFilename, nil
}

func DownloadTikTokVideo(url string) (string, error) {
	tmpDir := "./tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", errors.New("не удалось создать временную папку: " + err.Error())
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
	cmd := exec.Command(absYtDlpPath, "-f", "mp4", "-o", absFilename, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New("yt-dlp error: " + err.Error() + ", details: " + string(output))
	}
	if _, err := os.Stat(absFilename); err != nil {
		return "", errors.New("файл не был создан: " + err.Error() + ", yt-dlp output: " + string(output))
	}
	return absFilename, nil
}

func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[int64(i+os.Getpid()+n)%int64(len(letters))]
	}
	return string(b)
}
