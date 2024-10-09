package session

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/datarhei/core/v16/encoding/json"

	"github.com/stretchr/testify/require"
)

func TestVerifySession(t *testing.T) {
	jsondata := []byte(`{
		"match": "/memfs/6faad99a-c440-4df1-9344-963869718d8d/**",
		"remote": [
		  "foo.example.com"
		]
	  }`)

	var rawdata interface{}

	err := json.Unmarshal(jsondata, &rawdata)
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://foo.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://bar.example.com")
	require.Error(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-0000-963869718d8d/main.m3u8", "http://foo.example.com")
	require.Error(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "")
	require.Error(t, err)
}

func TestVerifySessionNoRemote(t *testing.T) {
	jsondata := []byte(`{
		"match": "/memfs/6faad99a-c440-4df1-9344-963869718d8d/**",
		"remote": []
	  }`)

	var rawdata interface{}

	err := json.Unmarshal(jsondata, &rawdata)
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://cm.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "")
	require.NoError(t, err)

	jsondata = []byte(`{
		"match": "/memfs/6faad99a-c440-4df1-9344-963869718d8d/**"
	  }`)

	err = json.Unmarshal(jsondata, &rawdata)
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://cm.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "")
	require.NoError(t, err)
}

func TestVerifySessionWildcardRemote(t *testing.T) {
	jsondata := []byte(`{
		"match": "/memfs/6faad99a-c440-4df1-9344-963869718d8d/**",
		"remote": [
		  "*.example.com"
		]
	  }`)

	var rawdata interface{}

	err := json.Unmarshal(jsondata, &rawdata)
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://foo.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://bar.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://sub.bar.example.com")
	require.Error(t, err)
}

func TestVerifySessionSuperWildcardRemote(t *testing.T) {
	jsondata := []byte(`{
		"match": "/memfs/6faad99a-c440-4df1-9344-963869718d8d/**",
		"remote": [
		  "**.example.com"
		]
	  }`)

	var rawdata interface{}

	err := json.Unmarshal(jsondata, &rawdata)
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://foo.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://bar.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://sub.bar.example.com")
	require.NoError(t, err)
}

func TestVerifySessionMultipleRemote(t *testing.T) {
	jsondata := []byte(`{
		"match": "/memfs/6faad99a-c440-4df1-9344-963869718d8d/**",
		"remote": [
		  "foo.example.com",
		  "bar.otherdomain.com"
		]
	  }`)

	var rawdata interface{}

	err := json.Unmarshal(jsondata, &rawdata)
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://foo.example.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://bar.otherdomain.com")
	require.NoError(t, err)

	_, err = verifySession(rawdata, "/memfs/6faad99a-c440-4df1-9344-963869718d8d/main.m3u8", "http://bar.example.com")
	require.Error(t, err)
}

func TestHeaderSize(t *testing.T) {
	header := http.Header{}

	header.Add("Content-Type", "application/json")
	header.Add("Content-Encoding", "gzip")

	buffer := &bytes.Buffer{}
	size := headerSize(header, buffer)

	require.Equal(t, "Content-Encoding: gzip\r\nContent-Type: application/json\r\n", buffer.String())
	require.Equal(t, int64(56), size)
}

func BenchmarkHeaderSize(b *testing.B) {
	header := http.Header{}

	header.Add("Content-Type", "application/json")
	header.Add("Content-Encoding", "gzip")

	buffer := &bytes.Buffer{}

	for i := 0; i < b.N; i++ {
		headerSize(header, buffer)
	}
}
