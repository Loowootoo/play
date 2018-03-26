package stores

import (
	"fmt"
	"net/http"
	"strings"

	"encoding/json"

	"errors"

	"regexp"

	"github.com/dave/flux"
	"github.com/dave/jsgo/server/messages"
	"github.com/dave/locstor"
	"github.com/dave/play/actions"
	"honnef.co/go/js/dom"
)

type LocalStore struct {
	app *App

	local *locstor.DataStore
}

func NewLocalStore(app *App) *LocalStore {
	s := &LocalStore{
		app:   app,
		local: locstor.NewDataStore(locstor.JSONEncoding),
	}
	return s
}

func (s *LocalStore) Handle(payload *flux.Payload) bool {
	switch action := payload.Action.(type) {
	case *actions.Load:
		var sizes []float64
		found, err := s.local.Find("split-sizes", &sizes)
		if err != nil {
			s.app.Fail(err)
			return true
		}
		if !found {
			sizes = defaultSizes
		}
		s.app.Dispatch(&actions.ChangeSplit{Sizes: sizes})

		location := strings.TrimPrefix(dom.GetWindow().Location().Pathname, "/")

		// No hash -> load files from local storage or use default files
		if location == "" {
			var current string
			var files map[string]string
			found, err = s.local.Find("files", &files)
			if err != nil {
				s.app.Fail(err)
				return true
			}
			if found {
				// if we found files in local storage, also load the current file
				if _, err := s.local.Find("current-file", &current); err != nil {
					s.app.Fail(err)
					return true
				}
			} else {
				files = defaultFiles
				current = defaultFile
			}
			s.app.Dispatch(&actions.LoadFiles{Files: files, Current: current})
			break
		}

		// Sha in hash -> load files from src.jsgo.io json blob
		if shaRegex.MatchString(location) {
			resp, err := http.Get(fmt.Sprintf("https://%s/%s.json", s.app.SrcHost(), location))
			if err != nil {
				s.app.Fail(err)
				return true
			}
			if resp.StatusCode != 200 {
				s.app.Fail(fmt.Errorf("error %d loading source", resp.StatusCode))
				return true
			}
			var m messages.Share
			if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
				s.app.Fail(err)
				return true
			}
			files, ok := m.Source["main"]
			if !ok {
				s.app.Fail(errors.New("package main not found in source"))
				return true
			}
			s.app.Dispatch(&actions.LoadFiles{Files: files})
			break
		}

		// Package path in hash -> open websocket and load files
		location = strings.TrimSuffix(location, "/")
		s.app.Dispatch(&actions.GetStart{Path: location})

	case *actions.UserChangedSplit:
		if err := s.local.Save("split-sizes", action.Sizes); err != nil {
			s.app.Fail(err)
			return true
		}
	case *actions.UserChangedText:
		payload.Wait(s.app.Editor)
		if err := s.saveFiles(); err != nil {
			s.app.Fail(err)
			return true
		}
	case *actions.UserChangedFile:
		payload.Wait(s.app.Editor)
		if err := s.saveCurrentFile(); err != nil {
			s.app.Fail(err)
			return true
		}
	case *actions.AddFile, *actions.DeleteFile:
		payload.Wait(s.app.Editor)
		if err := s.saveFiles(); err != nil {
			s.app.Fail(err)
			return true
		}
		if err := s.saveCurrentFile(); err != nil {
			s.app.Fail(err)
			return true
		}
	}
	return true
}

func (s *LocalStore) saveFiles() error {
	return s.local.Save("files", s.app.Editor.Files())
}
func (s *LocalStore) saveCurrentFile() error {
	return s.local.Save("current-file", s.app.Editor.Current())
}

var (
	defaultSizes = []float64{50, 50}
	defaultFile  = "main.go"
	defaultFiles = map[string]string{
		"main.go": `package main

import (
	"honnef.co/go/js/dom"
)

func main() {
    body := dom.GetWindow().Document().GetElementsByTagName("body")[0]
	body.SetInnerHTML("Hello, World! " + randnum())
}`,
		"rand.go": `package main

import (
    "fmt"
    "time"
    "math/rand"
)

func randnum() string {
    r := rand.Intn(10000)
    return fmt.Sprint(r)
}

func init() {
    rand.Seed(time.Now().UTC().UnixNano())
}`}
)

var shaRegex = regexp.MustCompile("^[0-9a-f]{40}$")
