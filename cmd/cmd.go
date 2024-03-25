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

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dvaumoron/gosince/config"
	"github.com/dvaumoron/gosince/versiondb"
	"github.com/spf13/cobra"
)

const (
	addedIn      = "added in"
	deprecatedIn = "and deprecated in"
	found        = "found"
)

var conf config.Config

func Init(version string) *cobra.Command {
	envRepoPath, envSourceUrl, err := config.InitDefault("GOSINCE_CACHE_PATH", "GOSINCE_SOURCE_URL")

	callGoDoc := false

	cmd := &cobra.Command{
		Use:   "gosince expr1 [expr2]",
		Short: "gosince shows the introducing version of a go package or symbol.",
		Long: `gosince shows the introducing version of a go package or symbol, find more details at : https://github.com/dvaumoron/gosince

Usage of gosince:
gosince <pkg>
gosince <sym>
gosince <pkg>.<sym>[.<methodOrField>]
gosince <pkg> <sym>[.<methodOrField>]
`,
		Version: version,
		Args:    cobra.RangeArgs(1, 2),
		Run: func(_ *cobra.Command, args []string) {
			if err != nil {
				fmt.Println(err)
				return
			}

			if conf.Verbose {
				fmt.Println("Use the repository", conf.RepoPath, "as local cache")
				fmt.Println("Use the url", conf.SourceUrl, "as base to download api information")
			}

			pkg, symbol := args[0], ""
			if len(args) == 1 {
				if index := strings.IndexByte(pkg, '.'); index != -1 {
					pkg, symbol = pkg[:index], pkg[index+1:]
				}
			} else {
				symbol = args[1]
			}

			versionDatas, err := versiondb.LoadDatas(conf)
			if err != nil {
				fmt.Println(err)
				return
			}

			pkg = strings.ToLower(pkg)
			symbol = strings.ToLower(symbol)
			symbolData, err := versionDatas.Since(pkg, symbol)
			if err != nil {
				query := ""
				switch err {
				case versiondb.ErrUnknownPackage:
					if symbol == "" {
						indexSlash := strings.IndexByte(pkg, '/')
						query = pkg[indexSlash+1:] // no error when indexSlash is -1
						break
					}
					fallthrough
				case versiondb.ErrUnknownSymbol:
					indexDot := strings.IndexByte(symbol, '.')
					query = symbol[indexDot+1:] // no error when indexDot is -1
				default:
					fmt.Println(err)
					return
				}

				results := versionDatas.Search(query)
				switch len(results) {
				case 0:
					fmt.Println(err)
					return
				case 1:
					result := results[0]
					if result[2] == "" {
						fmt.Println(found, result[0], addedIn, result[1])
					} else {
						fmt.Println(found, result[0], addedIn, result[1], deprecatedIn, result[2])
					}

					if callGoDoc {
						splitted := strings.Split(result[0], " ")
						if err = runGoDoc(splitted...); err != nil {
							fmt.Println(err)
							return
						}
					}
				default:
					fmt.Println("Several possibilities found :")
					for _, result := range results {
						if result[2] == "" {
							fmt.Println(result[0], addedIn, result[1])
						} else {
							fmt.Println(result[0], addedIn, result[1], deprecatedIn, result[2])
						}
					}
				}
				return
			}

			if symbolData[1] == "" {
				fmt.Println(addedIn, symbolData[0])
			} else {
				fmt.Println(addedIn, symbolData[0], deprecatedIn, symbolData[1])
			}

			if callGoDoc {
				if err = runGoDoc(args...); err != nil {
					fmt.Println(err)
				}
			}
		},
	}

	cmdFlags := cmd.Flags()
	cmdFlags.StringVarP(&conf.RepoPath, "cache-path", "p", envRepoPath, "Local path to cache the retrieved api information")
	cmdFlags.BoolVarP(&callGoDoc, "go-doc", "d", false, "Call go doc command")
	cmdFlags.StringVarP(&conf.SourceUrl, "source-addr", "a", envSourceUrl, "Location of Go source")
	cmdFlags.BoolVarP(&conf.Verbose, "verbose", "v", false, "Verbose output")

	return cmd
}

func runGoDoc(cmdArgs ...string) error {
	cmdArgs = append([]string{"doc"}, cmdArgs...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	return cmd.Run()
}
