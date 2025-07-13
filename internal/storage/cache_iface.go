package storage

type VideoCacheRepository interface {
	GetVideoFromCache(url string) (*VideoCache, error)
	SaveVideoToCache(url, telegramFileID string) error
	DeleteVideoFromCache(url string) error
	CleanOldCache(daysOld int) error
	GetCacheStats() (int, error)
}
