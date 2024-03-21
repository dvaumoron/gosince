/*
 *
 * Copyright 2024 gosince authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package config

import (
	"os"
	"path"
)

const defaultGoSourceUrl = "https://raw.githubusercontent.com/golang/go/master"

type Config struct {
	RepoPath  string
	SourceUrl string
	Verbose   bool
}

func InitDefault(envRepoPathName string, envRepoUrlName string) (string, string, error) {
	envRepoPath := os.Getenv(envRepoPathName)
	if envRepoPath == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		envRepoPath = path.Join(userHome, ".gosince")
	}

	envSourceUrl := os.Getenv(envRepoUrlName)
	if envSourceUrl == "" {
		envSourceUrl = defaultGoSourceUrl
	}
	return envRepoPath, envSourceUrl, nil
}
