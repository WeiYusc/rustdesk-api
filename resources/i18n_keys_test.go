package resources_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserFacingI18nKeysExistInAllLocales(t *testing.T) {
	locales, err := filepath.Glob("i18n/*.toml")
	if err != nil {
		t.Fatalf("glob locale files: %v", err)
	}
	if len(locales) == 0 {
		t.Fatal("expected locale files")
	}

	keys := []string{
		"NoCaptchaRequired",
		"UserDisabled",
		"UserNotFound",
		"LoginFailed",
		"LoginBanned",
		"UsernameExists",
		"LastAdminCannotDelete",
		"LastAdminCannotUpdate",
		"PasswordMismatch",
	}
	for _, locale := range locales {
		content, err := os.ReadFile(locale)
		if err != nil {
			t.Fatalf("read %s: %v", locale, err)
		}
		text := string(content)
		for _, key := range keys {
			if !strings.Contains(text, fmt.Sprintf("[%s]", key)) {
				t.Errorf("%s missing [%s]", locale, key)
			}
		}
	}
}
