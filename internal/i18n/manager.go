package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	tele "gopkg.in/telebot.v4"
)

// Manager управляет локализацией
type Manager struct {
	translations map[string]map[string]interface{}
	mutex        sync.RWMutex
	fallbackLang string
}

// NewManager создает новый менеджер локализации
func NewManager(fallbackLang string) *Manager {
	return &Manager{
		translations: make(map[string]map[string]interface{}),
		fallbackLang: fallbackLang,
	}
}

// LoadTranslations загружает переводы из файлов
func (m *Manager) LoadTranslations(translationsDir string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	fmt.Printf("[I18N] Загружаем переводы из директории: %s\n", translationsDir)

	// Читаем все файлы переводов из директории
	files, err := os.ReadDir(translationsDir)
	if err != nil {
		return fmt.Errorf("ошибка чтения директории переводов: %v", err)
	}

	fmt.Printf("[I18N] Найдено файлов в директории: %d\n", len(files))

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		lang := strings.TrimSuffix(file.Name(), ".json")
		filePath := filepath.Join(translationsDir, file.Name())

		fmt.Printf("[I18N] Загружаем язык: %s из файла: %s\n", lang, filePath)

		// Читаем файл перевода
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("ошибка чтения файла %s: %v", filePath, err)
		}

		// Парсим JSON
		var translations map[string]interface{}
		if err := json.Unmarshal(data, &translations); err != nil {
			return fmt.Errorf("ошибка парсинга JSON в файле %s: %v", filePath, err)
		}

		m.translations[lang] = translations
		fmt.Printf("[I18N] Загружено %d переводов для языка %s\n", len(translations), lang)
	}

	fmt.Printf("[I18N] Всего загружено языков: %d\n", len(m.translations))
	return nil
}

// GetUserLanguage определяет язык пользователя из Telegram
func (m *Manager) GetUserLanguage(user *tele.User) string {
	if user == nil {
		return m.fallbackLang
	}

	// Проверяем язык пользователя из Telegram
	if user.LanguageCode != "" {
		lang := strings.ToLower(user.LanguageCode)

		// Проверяем, есть ли перевод для этого языка
		if _, exists := m.translations[lang]; exists {
			return lang
		}

		// Если нет точного совпадения, пробуем найти по префиксу языка
		// Например, для "ru-RU" ищем "ru"
		if idx := strings.Index(lang, "-"); idx > 0 {
			baseLang := lang[:idx]
			if _, exists := m.translations[baseLang]; exists {
				return baseLang
			}
		}
	}

	return m.fallbackLang
}

// T возвращает переведенный текст для пользователя
func (m *Manager) T(user *tele.User, key string, args ...interface{}) string {
	lang := m.GetUserLanguage(user)

	m.mutex.RLock()
	translations, exists := m.translations[lang]
	m.mutex.RUnlock()

	if !exists {
		// Если перевод не найден, используем fallback
		m.mutex.RLock()
		translations, exists = m.translations[m.fallbackLang]
		m.mutex.RUnlock()

		if !exists {
			// Отладочная информация
			fmt.Printf("[I18N] Ошибка: нет переводов для языка %s и fallback %s\n", lang, m.fallbackLang)
			fmt.Printf("[I18N] Доступные языки: %v\n", m.GetAvailableLanguages())
			return key // Возвращаем ключ, если нет переводов вообще
		}
	}

	var textRaw interface{}
	var ok bool
	if textRaw, ok = translations[key]; !ok {
		// Если ключ не найден в текущем языке, ищем в fallback
		m.mutex.RLock()
		fallbackTranslations := m.translations[m.fallbackLang]
		m.mutex.RUnlock()
		textRaw, ok = fallbackTranslations[key]
		if !ok {
			fmt.Printf("[I18N] Ошибка: ключ '%s' не найден в языке %s и fallback %s\n", key, lang, m.fallbackLang)
			return key
		}
	}

	// Если перевод — массив строк, склеиваем через \n
	type templateData map[string]interface{}

	tmplApply := func(tmplStr string, data templateData) string {
		tmpl, err := template.New("").Parse(tmplStr)
		if err != nil {
			return tmplStr // если шаблон не парсится, возвращаем как есть
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, data)
		if err != nil {
			return tmplStr // если ошибка подстановки, возвращаем как есть
		}
		return buf.String()
	}

	switch v := textRaw.(type) {
	case string:
		if len(args) == 1 {
			if data, ok := args[0].(map[string]interface{}); ok {
				return tmplApply(v, data)
			}
		}
		if len(args) > 0 {
			return fmt.Sprintf(v, args...)
		}
		return v
	case []interface{}:
		lines := make([]string, 0, len(v))
		for _, line := range v {
			lineStr := fmt.Sprintf("%v", line)
			if len(args) == 1 {
				if data, ok := args[0].(map[string]interface{}); ok {
					lineStr = tmplApply(lineStr, data)
				}
			}
			lines = append(lines, lineStr)
		}
		joined := strings.Join(lines, "\n")
		if len(args) > 0 && (len(args) != 1 || !isMapStringInterface(args[0])) {
			return fmt.Sprintf(joined, args...)
		}
		return joined
	default:
		return fmt.Sprintf("[I18N] Некорректный тип перевода для ключа %s", key)
	}
}

func isMapStringInterface(arg interface{}) bool {
	_, ok := arg.(map[string]interface{})
	return ok
}

// GetAvailableLanguages возвращает список доступных языков
func (m *Manager) GetAvailableLanguages() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	languages := make([]string, 0, len(m.translations))
	for lang := range m.translations {
		languages = append(languages, lang)
	}
	return languages
}

// HasLanguage проверяет, есть ли переводы для указанного языка
func (m *Manager) HasLanguage(lang string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	_, exists := m.translations[lang]
	return exists
}
