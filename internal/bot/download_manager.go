package bot

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// NewDownloadManager создает новый менеджер скачиваний
func NewDownloadManager(maxWorkers int) *DownloadManager {
	return &DownloadManager{
		limiter:         make(chan struct{}, maxWorkers),
		mutexMap:        make(map[string]*sync.Mutex),
		mutexMutex:      sync.RWMutex{},
		activeDownloads: make(map[string]*DownloadInfo),
		downloadMutex:   sync.RWMutex{},
	}
}

// GetURLMutex возвращает мьютекс для конкретного URL
func (dm *DownloadManager) GetURLMutex(url string) *sync.Mutex {
	dm.mutexMutex.RLock()
	mutex, exists := dm.mutexMap[url]
	dm.mutexMutex.RUnlock()

	if !exists {
		dm.mutexMutex.Lock()
		// Проверяем еще раз после получения блокировки на запись
		if mutex, exists = dm.mutexMap[url]; !exists {
			mutex = &sync.Mutex{}
			dm.mutexMap[url] = mutex
		}
		dm.mutexMutex.Unlock()
	}

	return mutex
}

// CleanupURLMutex удаляет мьютекс для URL после завершения скачивания
func (dm *DownloadManager) CleanupURLMutex(url string) {
	dm.mutexMutex.Lock()
	delete(dm.mutexMap, url)
	dm.mutexMutex.Unlock()
}

// StartDownload регистрирует начало скачивания
func (dm *DownloadManager) StartDownload(url, requestID string, userID int64) *DownloadInfo {
	dm.downloadMutex.Lock()
	defer dm.downloadMutex.Unlock()

	downloadInfo := &DownloadInfo{
		RequestID: requestID,
		UserID:    userID,
		StartTime: time.Now(),
		Done:      make(chan struct{}),
	}

	dm.activeDownloads[url] = downloadInfo
	log.Printf("[DOWNLOAD] [%s] Зарегистрировано активное скачивание для URL: %s", requestID, url)

	return downloadInfo
}

// FinishDownload регистрирует завершение скачивания
func (dm *DownloadManager) FinishDownload(url string, err error) {
	dm.downloadMutex.Lock()
	defer dm.downloadMutex.Unlock()

	if downloadInfo, exists := dm.activeDownloads[url]; exists {
		downloadInfo.Error = err
		close(downloadInfo.Done)
		delete(dm.activeDownloads, url)
		log.Printf("[DOWNLOAD] [%s] Завершено скачивание для URL: %s (ошибка: %v)", downloadInfo.RequestID, url, err)
	}
}

// WaitForDownload ждет завершения активного скачивания
func (dm *DownloadManager) WaitForDownload(url string, timeout time.Duration) (*DownloadInfo, error) {
	dm.downloadMutex.RLock()
	downloadInfo, exists := dm.activeDownloads[url]
	dm.downloadMutex.RUnlock()

	if !exists {
		return nil, nil // Нет активного скачивания
	}

	log.Printf("[DOWNLOAD] Ожидание завершения скачивания URL: %s (начато пользователем %d)", url, downloadInfo.UserID)

	select {
	case <-downloadInfo.Done:
		return downloadInfo, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("таймаут ожидания скачивания")
	}
}

// IsDownloadActive проверяет, активно ли скачивание для URL
func (dm *DownloadManager) IsDownloadActive(url string) bool {
	dm.downloadMutex.RLock()
	defer dm.downloadMutex.RUnlock()

	_, exists := dm.activeDownloads[url]
	return exists
}

// GetActiveDownloads возвращает список активных скачиваний
func (dm *DownloadManager) GetActiveDownloads() map[string]*DownloadInfo {
	dm.downloadMutex.RLock()
	defer dm.downloadMutex.RUnlock()

	result := make(map[string]*DownloadInfo)
	for url, info := range dm.activeDownloads {
		result[url] = info
	}
	return result
}

// AcquireDownloadSlot пытается получить слот для скачивания
func (dm *DownloadManager) AcquireDownloadSlot() bool {
	select {
	case dm.limiter <- struct{}{}:
		return true
	default:
		return false
	}
}

// ReleaseDownloadSlot освобождает слот для скачивания
func (dm *DownloadManager) ReleaseDownloadSlot() {
	select {
	case <-dm.limiter:
	default:
	}
}
