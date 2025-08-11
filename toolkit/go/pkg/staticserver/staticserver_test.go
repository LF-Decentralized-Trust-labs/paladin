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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LF-Decentralized-Trust-labs/paladin/config/pkg/pldconf"
)

// Helper to create a temporary UI directory with files for testing
func setupUITestDir(t *testing.T, relativeDir string) string {
	tmpDir := t.TempDir()
	tmpDirP := filepath.Join(tmpDir, relativeDir)
	_ = os.Mkdir(tmpDirP, 0755)
	_ = os.WriteFile(filepath.Join(tmpDirP, "index.html"), []byte("<html><body>Some content</body></html>"), 0644)
	return tmpDirP
}

func TestServeStaticFile(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, conf.URLPath, nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<html><body>Some content</body></html>")
}

func TestServeStaticPath(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, conf.URLPath+"/", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<html><body>Some content</body></html>")
}

func TestServeStaticBaseRedirect(t *testing.T) {
	p := "/somepath"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:      true,
		StaticPath:   tmpDir,
		URLPath:      p,
		BaseRedirect: "somepath/index.html",
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, conf.URLPath, nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	require.Equal(t, http.StatusFound, res.Code)
	require.Equal(t, "somepath/index.html", res.Header().Get("Location"))
}

func TestServeStaticNoRedirectWithSlash(t *testing.T) {
	p := "/somepath"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:      true,
		StaticPath:   tmpDir,
		URLPath:      p,
		BaseRedirect: "somepath/index.html",
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, "/somepath/", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
}

func TestServeStaticFileNotFound(t *testing.T) {
	p := "/static"

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: "/nonexistent",
		URLPath:    p,
	}
	server := NewStaticServer(conf)

	req := httptest.NewRequest(http.MethodGet, conf.URLPath, nil)
	rr := httptest.NewRecorder()

	// Run the handler
	server.HTTPHandler(rr, req)

	assert.Equal(t, 404, rr.Result().StatusCode)
}

func TestServeMissingFile(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p + "/nonexistent.html",
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, p, nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<html><body>Some content</body></html>")
}

func TestServeBadRequest(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, p+"/../../invalid", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestUIServerErrorHandling(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, "/../../invalid", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
}

func TestServeInternalServerError(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	// Modify osStat to simulate a non-NotExist error
	originalStat := osStat
	osStat = func(path string) (os.FileInfo, error) {
		return nil, errors.New("mock server error")
	}
	defer func() { osStat = originalStat }()

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, p+"/error-file", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)
	require.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Contains(t, res.Body.String(), "HTTP server failed to load index file")
}

func TestServeFilePathRelError(t *testing.T) {
	// Set up a StaticServer with a URL prefix that causes filepath.Rel to fail
	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: "/static",
		URLPath:    "/invalid/prefix",
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, "/different/prefix", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	assert.Equal(t, http.StatusNotFound, res.Code)
}

func TestServeCSSFile(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	// Create a CSS file in the temporary directory
	err := os.WriteFile(filepath.Join(tmpDir, "style.css"), []byte("body { background: #FFF; }"), 0644)
	require.NoError(t, err)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)
	req := httptest.NewRequest(http.MethodGet, p+"/style.css", nil)
	res := httptest.NewRecorder()

	server.HTTPHandler(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "body { background: #FFF; }")
}

func TestJSONEncodingError(t *testing.T) {
	p := "/static"
	tmpDir := setupUITestDir(t, p)

	conf := pldconf.StaticServerConfig{
		Enabled:    true,
		StaticPath: tmpDir,
		URLPath:    p,
	}
	server := NewStaticServer(conf)

	// Create a ResponseWriter that will simulate an encoding error
	brokenWriter := &brokenResponseWriter{}
	req := httptest.NewRequest(http.MethodGet, p+"/error-file", nil)

	server.HTTPHandler(brokenWriter, req)

	// No need to assert anything on brokenWriter; just ensuring no panics occur.
}

type brokenResponseWriter struct{}

func (brw *brokenResponseWriter) Header() http.Header        { return http.Header{} }
func (brw *brokenResponseWriter) Write([]byte) (int, error)  { return 0, errors.New("write error") }
func (brw *brokenResponseWriter) WriteHeader(statusCode int) {}
