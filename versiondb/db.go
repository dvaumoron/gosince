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

package versiondb

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/dvaumoron/gosince/config"
)

const go1Dot = "go1."

var (
	errParsingClosePar   = errors.New("parsing failure : wait close parenthesis")
	errParsingComma      = errors.New("parsing failure : no comma separator")
	errParsingOpenPar    = errors.New("parsing failure : wait open parenthesis")
	errParsingSeparator  = errors.New("parsing failure : no field separator")
	errParsingSpace      = errors.New("parsing failure : wait space separator")
	errParsingStart      = errors.New("parsing failure : wrong start")
	errParsingSymbolType = errors.New("parsing failure : unknown symbol type")
	errUnexistingVersion = errors.New("can not retrieve go1 information") // inner string only displayed for go1, else used as marker.
	ErrUnknownPackage    = errors.New("package not found")
	ErrUnknownSymbol     = errors.New("symbol not found")
)

type VersionDatas struct {
	data  map[string]map[string]string
	index map[string][][2]string
}

func LoadDatas(conf config.Config) (VersionDatas, error) {
	repobase := path.Join(conf.RepoPath, go1Dot)
	sourceBase, err := url.JoinPath(conf.SourceUrl, "api", go1Dot)
	if err != nil {
		return VersionDatas{}, err
	}

	dl := dataLoader{
		VersionDatas: VersionDatas{data: map[string]map[string]string{}, index: map[string][][2]string{}},
		repobase:     repobase, sourceBase: sourceBase, verbose: conf.Verbose,
	}

	return dl.VersionDatas, dl.load()
}

func (vd VersionDatas) Search(key string) [][2]string {
	return vd.index[key]
}

func (vd VersionDatas) Since(pkg string, symbol string) (string, error) {
	pkgSymbols, ok := vd.data[pkg]
	if !ok {
		return "", ErrUnknownPackage
	}

	since, ok := pkgSymbols[symbol] // pkgSymbols must contains ""
	if !ok {
		return "", ErrUnknownSymbol
	}
	return since, nil
}

type dataLoader struct {
	VersionDatas
	repobase   string
	sourceBase string
	verbose    bool
}

func (dl dataLoader) addIndexEntry(key string, entry string, version string) {
	dl.index[key] = append(dl.index[key], [2]string{entry, version})
}

func (dl dataLoader) addIndexPackageEntry(pkg string, version string) {
	indexSlash := strings.LastIndexByte(pkg, '/')
	dl.addIndexEntry(pkg[indexSlash+1:], pkg, version) // no error when indexSlash is -1
}

func (dl dataLoader) addIndexSymbolEntry(pkg string, symbol string, version string) {
	var entryBuilder strings.Builder
	entryBuilder.WriteString(pkg)
	entryBuilder.WriteByte(' ')
	entryBuilder.WriteString(symbol)

	indexDot := strings.LastIndexByte(symbol, '.')
	dl.addIndexEntry(strings.ToLower(symbol[indexDot+1:]), entryBuilder.String(), version) // no error when indexDot is -1
}

func (dl dataLoader) load() error {
	versionData, err := dl.read("txt")
	if err != nil {
		return err
	}

	err = dl.parseVersionData("go1", versionData)
	if err != nil {
		return err
	}

	for minorVersion := 1; true; minorVersion++ {
		minorVersionStr := strconv.Itoa(minorVersion)
		versionData, err = dl.read(minorVersionStr + ".txt")
		if err != nil {
			if err == errUnexistingVersion {
				return nil
			}
			return err
		}

		err = dl.parseVersionData(go1Dot+minorVersionStr, versionData)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dl dataLoader) parseVersionData(version string, versionData []byte) error {
	// TODO improve
	versionDataScanner := bufio.NewScanner(bytes.NewReader(versionData))
	for versionDataScanner.Scan() {
		line := versionDataScanner.Text()
		if indexSharp := strings.IndexByte(line, '#'); indexSharp != -1 {
			// cut comment
			if indexSharp == 0 {
				continue
			}
			line = line[:indexSharp]
		}

		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		if trimmedLine[len(trimmedLine)-12:] == "//deprecated" {
			// TODO register and display deprecation version
			continue
		}

		lineWithoutPrefix, ok := strings.CutPrefix(trimmedLine, "pkg ")
		if !ok {
			return errParsingStart
		}

		indexComma := strings.IndexByte(lineWithoutPrefix, ',')
		if indexComma == -1 {
			return errParsingComma
		}

		pkg := lineWithoutPrefix[:indexComma]
		pkgSymbols, ok := dl.data[pkg]
		if !ok {
			pkgSymbols = map[string]string{"": version} // allows search of package version with ""
			dl.data[pkg] = pkgSymbols
			dl.addIndexPackageEntry(pkg, version)
		}

		symbol := ""
		symbolDesc := lineWithoutPrefix[indexComma+2:] // ignore comma and space
		indexSpace := strings.IndexByte(symbolDesc, ' ')
		if indexSpace == -1 {
			return errParsingSpace
		}

		symbolDescWithoutPrefix := symbolDesc[indexSpace+1:]
		switch symbolDesc[:indexSpace] {
		case "const", "var":
			indexSpace = strings.IndexByte(symbolDescWithoutPrefix, ' ')
			if indexSpace == -1 {
				return errParsingSpace
			}

			symbol = symbolDescWithoutPrefix[:indexSpace]
		case "func":
			indexParent := strings.IndexByte(symbolDescWithoutPrefix, '(')
			if indexParent == -1 {
				return errParsingOpenPar
			}

			symbol = nameWithoutGeneric(symbolDescWithoutPrefix[:indexParent])
		case "method":
			indexStart := 1
			if symbolDescWithoutPrefix[1] == '*' {
				indexStart = 2
			}

			indexParent := strings.IndexByte(symbolDescWithoutPrefix, ')')
			if indexParent == -1 {
				return errParsingClosePar
			}

			methodDescWithoutReceiver := symbolDescWithoutPrefix[indexParent+2:] // ignore close parenthesis and space
			indexParent2 := strings.IndexByte(methodDescWithoutReceiver, '(')
			if indexParent2 == -1 {
				return errParsingOpenPar
			}

			var symbolBuilder strings.Builder
			symbolBuilder.WriteString(symbolDescWithoutPrefix[indexStart:indexParent])
			symbolBuilder.WriteByte('.')
			symbolBuilder.WriteString(methodDescWithoutReceiver[:indexParent2])
			symbol = symbolBuilder.String()
		case "type":
			indexSpace = strings.IndexByte(symbolDescWithoutPrefix, ' ')
			if indexSpace == -1 {
				return errParsingSpace
			}

			symbol = nameWithoutGeneric(symbolDescWithoutPrefix[:indexSpace])

			kindInterface := true
			indexComma := 0
			symbolDescWithoutPrefixLen := len(symbolDescWithoutPrefix)
			indexInterface := strings.Index(symbolDescWithoutPrefix, "interface")
			if indexInterface == -1 {
				indexStruct := strings.Index(symbolDescWithoutPrefix, "struct")
				if indexStruct == -1 {
					break
				} else {
					kindInterface = false
					indexComma = indexStruct + 6
					if indexComma == symbolDescWithoutPrefixLen || symbolDescWithoutPrefix[indexComma] != ',' {
						break
					}
				}
			} else {
				indexComma = indexInterface + 9
				if indexComma == symbolDescWithoutPrefixLen || symbolDescWithoutPrefix[indexComma] != ',' {
					break
				}
			}

			indexSeparator := 0
			fieldDesc := symbolDescWithoutPrefix[indexComma+2:] // ignore comma and space
			if kindInterface {
				if fieldDesc == "unexported methods" {
					break
				}

				indexSeparator = strings.IndexByte(fieldDesc, '(')
			} else {
				indexSeparator = strings.IndexByte(fieldDesc, ' ')
			}
			if indexSeparator == -1 {
				return errParsingSeparator
			}

			var symbolBuilder strings.Builder
			symbolBuilder.WriteString(nameWithoutGeneric(symbolDescWithoutPrefix[:indexSpace]))
			symbolBuilder.WriteByte('.')
			symbolBuilder.WriteString(fieldDesc[:indexSeparator])
			symbol = symbolBuilder.String()
		default:
			return errParsingSymbolType
		}

		dl.register(pkgSymbols, pkg, symbol, version)
	}
	return versionDataScanner.Err()
}

func (dl dataLoader) read(fileEnd string) ([]byte, error) {
	filePath := dl.repobase + fileEnd
	data, err := os.ReadFile(filePath)
	if err == nil {
		return data, nil
	}

	if dl.verbose {
		fmt.Println("Failed to read", filePath, ":", err)
	}

	fileURL := dl.sourceBase + fileEnd
	if data, err = download(fileURL); err != nil {
		return nil, err
	}

	if strings.TrimSpace(string(data)) == "404: Not Found" {
		return nil, errUnexistingVersion
	}
	return data, writeFile(filePath, data)
}

func (dl dataLoader) register(pkgSymbols map[string]string, pkg string, symbol string, version string) {
	symbolLower := strings.ToLower(symbol)
	if _, ok := pkgSymbols[symbolLower]; ok { // no override
		return
	}

	pkgSymbols[symbolLower] = version
	dl.addIndexSymbolEntry(pkg, symbol, version)
}

func download(dURL string) ([]byte, error) {
	resp, err := http.Get(dURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// supposing file will not be "too big"
	return io.ReadAll(resp.Body)
}

func nameWithoutGeneric(name string) string {
	indexSquare := strings.IndexByte(name, '[')
	if indexSquare == -1 {
		return name
	}
	return name[:indexSquare]
}

// Create the parents directories if needed and write the file
func writeFile(path string, data []byte) error {
	if index := strings.LastIndexByte(path, '/'); index != -1 {
		if err := os.MkdirAll(path[:index], 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0644)
}
