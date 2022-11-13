package controllers

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
)

func getGithubTokenOrSkip() string {
	token, run := os.LookupEnv("GITHUB_TOKEN")
	if !run {
		Skip("'GITHUB_TOKEN' environment variable is not set")
	}
	return token
}
