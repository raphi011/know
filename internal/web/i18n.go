package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
)

//go:embed messages/*.json
var messagesFS embed.FS

// messages holds all loaded translations: locale → key → value.
var messages map[string]map[string]string

func init() {
	messages = make(map[string]map[string]string)
	for _, locale := range []string{"en", "de"} {
		data, err := messagesFS.ReadFile("messages/" + locale + ".json")
		if err != nil {
			slog.Error("failed to load i18n messages", "locale", locale, "error", err)
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			slog.Error("failed to parse i18n messages", "locale", locale, "error", err)
			continue
		}
		messages[locale] = m
	}
}

// T returns a translation function for the given locale.
// Falls back to English, then returns the key itself.
func T(locale string) func(string) string {
	return func(key string) string {
		if m, ok := messages[locale]; ok {
			if v, ok := m[key]; ok {
				return v
			}
		}
		// Fallback to English
		if locale != "en" {
			if m, ok := messages["en"]; ok {
				if v, ok := m[key]; ok {
					return v
				}
			}
		}
		return fmt.Sprintf("[%s]", key)
	}
}
