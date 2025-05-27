// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package staticserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/kaleido-io/paladin/common/go/pkg/i18n"
	"github.com/kaleido-io/paladin/common/go/pkg/log"
	"github.com/kaleido-io/paladin/common/go/pkg/pldmsgs"
	"github.com/kaleido-io/paladin/config/pkg/pldconf"
)

// used to allow mocking of os.Stat in tests
var osStat = os.Stat

type StaticServer interface {
	HTTPHandler(w http.ResponseWriter, r *http.Request)
}

var _ StaticServer = &staticServer{}

type staticServer struct {
	staticPath   string // path to static files
	indexPath    string // path to the index file (relative to staticPath)
	urlPrefix    string // prefix for the URL to serve static files from
	baseRedirect string // redirect when no path
}

func NewStaticServer(conf pldconf.StaticServerConfig) *staticServer {
	return &staticServer{
		staticPath:   conf.StaticPath,
		indexPath:    "index.html",
		urlPrefix:    conf.URLPath,
		baseRedirect: conf.BaseRedirect,
	}
}

func (s *staticServer) HTTPHandler(w http.ResponseWriter, r *http.Request) {
	s.httpHandler(w, r)
}

// serveHTTP serves the static files in the ui directory
func (s *staticServer) httpHandler(w http.ResponseWriter, r *http.Request) {

	if r.URL.Path == s.urlPrefix && s.baseRedirect != "" {
		w.Header().Set("Location", s.baseRedirect)
		w.WriteHeader(http.StatusFound)
		return
	}

	path, _ := filepath.Rel(s.urlPrefix, r.URL.Path)

	// prepend the path with the path to the static directory
	path = filepath.Join(s.staticPath, strings.ReplaceAll(path, "..", "_"), "/")

	// check whether a file exists at the given path
	if _, err := osStat(path); os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(s.staticPath, s.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		log.L(r.Context()).Errorf("Failed to serve file: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(&fftypes.RESTError{
			Error: i18n.ExpandWithCode(r.Context(), i18n.MessageKey(pldmsgs.MsgUIServerFailed)),
		})
		return
	}

	// Serve the requested file directly
	http.StripPrefix(s.urlPrefix, http.FileServer(http.Dir(s.staticPath))).ServeHTTP(w, r)
}
