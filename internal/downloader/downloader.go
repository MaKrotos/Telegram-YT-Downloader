package downloader

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func DownloadYouTubeVideo(url string) (string, error) {
	filename := filepath.Join(os.TempDir(), "ytvideo_"+randomString(8)+".mp4")
	var ytDlpPath string
	if runtime.GOOS == "windows" {
		ytDlpPath = "./yt-dlp.exe"
	} else {
		ytDlpPath = "./yt-dlp_linux"
	}
	cmd := exec.Command(ytDlpPath, "-f", "mp4", "-o", filename, url)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", errors.New("yt-dlp error: " + err.Error() + ", details: " + string(output))
	}
	if _, err := os.Stat(filename); err != nil {
		return "", errors.New("файл не был создан: " + err.Error())
	}
	return filename, nil
}

func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[int64(i+os.Getpid()+n)%int64(len(letters))]
	}
	return string(b)
}
