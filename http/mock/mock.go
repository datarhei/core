package mock

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/errorhandler"
	"github.com/datarhei/core/v16/http/validator"
	"github.com/datarhei/core/v16/internal/testhelper"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/resources"
	"github.com/datarhei/core/v16/resources/psutil"
	"github.com/datarhei/core/v16/restream"
	jsonstore "github.com/datarhei/core/v16/restream/store/json"

	"github.com/invopop/jsonschema"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
)

func DummyRestreamer(pathPrefix string) (restream.Restreamer, error) {
	binary, err := testhelper.BuildBinary("ffmpeg", filepath.Join(pathPrefix, "../../internal/testhelper"))
	if err != nil {
		return nil, fmt.Errorf("failed to build helper program: %w", err)
	}

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create memory filesystem: %w", err)
	}

	store, err := jsonstore.New(jsonstore.Config{
		Filesystem: memfs,
	})
	if err != nil {
		return nil, err
	}

	psutil, err := psutil.New("", nil)
	if err != nil {
		return nil, err
	}

	resources, err := resources.New(resources.Config{
		PSUtil: psutil,
	})
	if err != nil {
		return nil, err
	}

	ffmpeg, err := ffmpeg.New(ffmpeg.Config{
		Binary:           binary,
		MaxLogLines:      100,
		LogHistoryLength: 3,
		Resource:         resources,
	})
	if err != nil {
		return nil, err
	}

	rs, err := restream.New(restream.Config{
		Store:       store,
		FFmpeg:      ffmpeg,
		Filesystems: []fs.Filesystem{memfs},
	})
	if err != nil {
		return nil, err
	}

	return rs, nil
}

func DummyEcho() *echo.Echo {
	router := echo.New()
	router.HideBanner = true
	router.HidePort = true
	router.HTTPErrorHandler = errorhandler.HTTPErrorHandler
	router.Logger.SetOutput(io.Discard)
	router.Validator = validator.New()

	return router
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Raw     []byte
	Data    interface{}
}

func Request(t require.TestingT, httpstatus int, router *echo.Echo, method, path string, data io.Reader) *Response {
	return RequestEx(t, httpstatus, router, method, path, data, true)
}

func RequestEx(t require.TestingT, httpstatus int, router *echo.Echo, method, path string, data io.Reader, checkResponse bool) *Response {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, data)
	if data != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	router.ServeHTTP(w, req)

	var response *Response = nil

	if checkResponse {
		response = CheckResponse(t, w.Result())
	} else {
		response = CheckResponseMinimal(t, w.Result())
	}

	require.Equal(t, httpstatus, w.Code, string(response.Raw))

	return response
}

func CheckResponseMinimal(t require.TestingT, res *http.Response) *Response {
	response := &Response{
		Code: res.StatusCode,
	}

	res.Body.Close()

	return response
}

func CheckResponse(t require.TestingT, res *http.Response) *Response {
	response := &Response{
		Code: res.StatusCode,
	}

	body, err := io.ReadAll(res.Body)
	require.Equal(t, nil, err)

	response.Raw = body

	if strings.Contains(res.Header.Get("Content-Type"), "application/json") {
		err := json.Unmarshal(body, &response.Data)
		require.Equal(t, nil, err)
	} else {
		response.Data = body
	}

	if response.Code != http.StatusOK {
		if err, ok := response.Data.(api.Error); ok {
			response.Message = err.Message
		}
	}

	return response
}

func Validate(t require.TestingT, datatype, data interface{}) bool {
	schema, err := jsonschema.Reflect(datatype).MarshalJSON()
	require.NoError(t, err)

	schemaLoader := gojsonschema.NewStringLoader(string(schema))
	documentLoader := gojsonschema.NewGoLoader(data)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	require.Equal(t, nil, err)
	require.Equal(t, true, result.Valid(), result.Errors())

	return true
}

func Read(t require.TestingT, path string) io.Reader {
	data, err := os.ReadFile(path)
	require.Equal(t, nil, err)

	return bytes.NewReader(data)
}
