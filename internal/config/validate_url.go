package config

import (
	"fmt"
	"net/url"
)

func validateURL(urlStr string, urlName string) error {
	if urlStr == "" {
		return fmt.Errorf("%s must be a valid URL but is empty", urlName)
	}

	u, err := url.Parse(urlStr)

	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", urlName, err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must use http:// or https:// scheme", urlName)
	}

	return nil
}
