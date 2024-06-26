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

import "errors"

var (
	errParsingClosing           = errors.New("parsing failure : wait closing separator")
	errParsingString            = errors.New("parsing failure : unended string")
	errParsingThirdPart         = errors.New("parsing failure : unhandled third part in definition")
	errParsingUnexpectedClosing = errors.New("parsing failure : unexpected closing separator")
	errParsingWrongClosing      = errors.New("parsing failure : wait another closing separator")
)

type node interface {
	cast() (string, []node)
}

type listNode []node

func (l listNode) cast() (string, []node) {
	return "", l
}

type stringNode string

func (s stringNode) cast() (string, []node) {
	return string(s), nil
}

func appendBuffer(splitted []node, buffer []rune) ([]node, []rune) {
	if len(buffer) != 0 {
		splitted = append(splitted, stringNode(buffer))
		buffer = buffer[:0]
	}
	return splitted, buffer
}

func consumeString(chars <-chan rune, delim rune) stringNode {
	var buffer []rune
	for char := range chars {
		switch char {
		case delim:
			return stringNode(buffer)
		case '\\':
			if char2, ok := <-chars; ok {
				buffer = append(buffer, char, char2)
			}
		default:
			buffer = append(buffer, char)
		}
	}
	panic(errParsingString)
}

func sendChar(chars chan<- rune, line string) {
	for _, char := range line {
		chars <- char
	}
	close(chars)
}

func smartSplit(line string) ([]node, []node) {
	chars := make(chan rune)
	go sendChar(chars, line)

	var buffer []rune
	var splitted, splitted2 []node
	for char := range chars {
		switch char {
		case '"', '\'':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, consumeString(chars, char))
		case '(':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, ')'))
		case '[':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, ']'))
		case '{':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, '}'))
		case ')', ']', '}':
			panic(errParsingUnexpectedClosing)
		case ',':
			break
		case ' ':
			splitted, buffer = appendBuffer(splitted, buffer)
		default:
			buffer = append(buffer, char)
		}
	}

	splitted, _ = appendBuffer(splitted, buffer)
	splitted2 = splitSecond(chars)
	return splitted, splitted2
}

func splitSecond(chars <-chan rune) []node {
	var buffer []rune
	var splitted []node
	for char := range chars {
		switch char {
		case '"', '\'':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, consumeString(chars, char))
		case '(':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, ')'))
		case '[':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, ']'))
		case '{':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, '}'))
		case ')', ']', '}':
			panic(errParsingWrongClosing)
		case ',':
			panic(errParsingThirdPart)
		case ' ':
			splitted, buffer = appendBuffer(splitted, buffer)
		default:
			buffer = append(buffer, char)
		}
	}

	splitted, _ = appendBuffer(splitted, buffer)
	return splitted
}

func splitSub(chars <-chan rune, delim rune) listNode {
	var buffer []rune
	var splitted []node
	for char := range chars {
		switch char {
		case delim: // no error on duplicate
			splitted, _ = appendBuffer(splitted, buffer)
			return splitted
		case '"', '\'':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, consumeString(chars, char))
		case '(':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, ')'))
		case '[':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, ']'))
		case '{':
			splitted, buffer = appendBuffer(splitted, buffer)
			splitted = append(splitted, splitSub(chars, '}'))
		case ')', ']', '}':
			panic(errParsingWrongClosing)
		case ',', ' ':
			splitted, buffer = appendBuffer(splitted, buffer)
		default:
			buffer = append(buffer, char)
		}
	}
	panic(errParsingClosing)
}
