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
	errParsingComma        = errors.New("parsing failure : no comma separator")
	errParsingMethod       = errors.New("parsing failure : empty method")
	errParsingMethodName   = errors.New("parsing failure : empty method name")
	errParsingName         = errors.New("parsing failure : empty name")
	errParsingReceiver     = errors.New("parsing failure : empty receiver")
	errParsingReceiverName = errors.New("parsing failure : empty receiver name")
	errParsingStart        = errors.New("parsing failure : wrong start")
	errParsingSubName      = errors.New("parsing failure : empty field or method name")
	errParsingType = errors.New("parsing failure : unknown definition type")
	errParsingUncomplete   = errors.New("parsing failure : not enough element in definition")
	errUnexistingVersion   = errors.New("can not retrieve go1 information") // inner string only displayed for go1, else used as marker.
	ErrUnknownPackage      = errors.New("package not found")
	ErrUnknownSymbol       = errors.New("symbol not found")
)

type VersionDatas struct {
	data  map[string]map[string][2]string
	index map[string][][3]string
}

func LoadDatas(conf config.Config) (VersionDatas, error) {
	repobase := path.Join(conf.RepoPath, go1Dot)
	sourceBase, err := url.JoinPath(conf.SourceUrl, "api", go1Dot)
	if err != nil {
		return VersionDatas{}, err
	}

	dl := dataLoader{
		VersionDatas: VersionDatas{data: map[string]map[string][2]string{}, index: map[string][][3]string{}},
		repobase:     repobase, sourceBase: sourceBase, verbose: conf.Verbose,
	}

	return dl.VersionDatas, dl.load()
}

func (vd VersionDatas) Search(key string) [][3]string {
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
	return since[0], nil
}

type dataLoader struct {
	VersionDatas
	repobase   string
	sourceBase string
	verbose    bool
}

func (dl dataLoader) addIndexEntry(key string, entry string, version string, deprecated bool) {
	if deprecated {
		for currentIndex, indexEntry := range dl.index[key] {
			if indexEntry[0] == entry {
				indexEntry[2] = version
				dl.index[key][currentIndex] = indexEntry
				break
			}
		}
	} else {
		dl.index[key] = append(dl.index[key], [3]string{entry, version})
	}
}

func (dl dataLoader) addIndexPackageEntry(pkg string, version string) {
	indexSlash := strings.LastIndexByte(pkg, '/')
	dl.addIndexEntry(pkg[indexSlash+1:], pkg, version, false) // no error when indexSlash is -1
}

func (dl dataLoader) addIndexSymbolEntry(pkg string, symbol string, version string, deprecated bool) {
	var entryBuilder strings.Builder
	entryBuilder.WriteString(pkg)
	entryBuilder.WriteByte(' ')
	entryBuilder.WriteString(symbol)

	indexDot := strings.LastIndexByte(symbol, '.')
	dl.addIndexEntry(strings.ToLower(symbol[indexDot+1:]), entryBuilder.String(), version, deprecated) // no error when indexDot is -1
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

		lenMinus12 := len(trimmedLine) - 12
		deprecated := trimmedLine[lenMinus12:] == "//deprecated"
		if deprecated {
			trimmedLine = trimmedLine[:lenMinus12]
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
			pkgSymbols = map[string][2]string{"": {version}} // allows search of package version with ""
			dl.data[pkg] = pkgSymbols
			dl.addIndexPackageEntry(pkg, version)
		}

		symbolDesc := lineWithoutPrefix[indexComma+2:] // ignore comma and space
		firstPart, secondPart := smartSplit(symbolDesc)
		if len(firstPart) < 2 {
			return errParsingUncomplete
		}

		symbol := ""
		switch symbolType, _ := firstPart[0].cast(); symbolType {
		case "const", "func", "var":
			symbol, _ = firstPart[1].cast()
			if symbol == "" {
				return errParsingName
			}
		case "method":
			if len(firstPart) < 3 {
				return errParsingMethod
			}

			_, receiver := firstPart[1].cast()
			if len(receiver) == 0 {
				return errParsingReceiver
			}

			typeName, _ := receiver[0].cast()
			if typeName == "" {
				return errParsingReceiverName
			}
			if typeName[0] == '*' {
				typeName = typeName[1:]
			}

			methodName, _ := firstPart[2].cast()
			if methodName == "" {
				return errParsingMethodName
			}

			var symbolBuilder strings.Builder
			symbolBuilder.WriteString(typeName)
			symbolBuilder.WriteByte('.')
			symbolBuilder.WriteString(methodName)
			symbol = symbolBuilder.String()
		case "type":
			symbol, _ = firstPart[1].cast()
			if symbol == "" {
				return errParsingName
			}

			if len(secondPart) == 0 {
				break
			}

			subName, _ := secondPart[0].cast()
			if subName == "" {
				return errParsingSubName
			}

			var symbolBuilder strings.Builder
			symbolBuilder.WriteString(symbol)
			symbolBuilder.WriteByte('.')
			symbolBuilder.WriteString(subName)
			symbol = symbolBuilder.String()
		default:
			return errParsingType
		}

		dl.register(pkgSymbols, pkg, symbol, version, deprecated)
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

func (dl dataLoader) register(pkgSymbols map[string][2]string, pkg string, symbol string, version string, deprecated bool) {
	symbolLower := strings.ToLower(symbol)
	if deprecated {
		symbolData := pkgSymbols[symbolLower]
		symbolData[1] = version
		pkgSymbols[symbolLower] = symbolData
	} else {
		if _, ok := pkgSymbols[symbolLower]; ok { // no override
			return
		}

		pkgSymbols[symbolLower] = [2]string{version}
	}
	dl.addIndexSymbolEntry(pkg, symbol, version, deprecated)
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

// Create the parents directories if needed and write the file
func writeFile(path string, data []byte) error {
	if index := strings.LastIndexByte(path, '/'); index != -1 {
		if err := os.MkdirAll(path[:index], 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0644)
}
