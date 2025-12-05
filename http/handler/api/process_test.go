package api

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/datarhei/core/v16/encoding/json"
	"github.com/datarhei/core/v16/http/api"
	"github.com/datarhei/core/v16/http/mock"
	"github.com/datarhei/core/v16/iam"
	"github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	"github.com/datarhei/core/v16/internal/mock/restream"
	"github.com/datarhei/core/v16/io/fs"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"
)

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    interface{}
}

func getDummyRestreamHandler() (*ProcessHandler, error) {
	rs, err := restream.New(nil, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create memory filesystem: %w", err)
	}

	policyAdapter, err := policy.NewJSONAdapter(memfs, "./policy.json", nil)
	if err != nil {
		return nil, err
	}

	identityAdapter, err := identity.NewJSONAdapter(memfs, "./users.json", nil)
	if err != nil {
		return nil, err
	}

	iam, err := iam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser: identity.User{
			Name: "foobar",
		},
		JWTRealm:  "",
		JWTSecret: "",
		Logger:    nil,
	})
	if err != nil {
		return nil, err
	}

	iam.AddPolicy("$anon", "$none", []string{"api"}, "/**", []string{"ANY"})
	iam.AddPolicy("$anon", "$none", []string{"fs"}, "/**", []string{"ANY"})
	iam.AddPolicy("$anon", "$none", []string{"process"}, "**", []string{"ANY"})

	handler := NewProcess(rs, iam)

	return handler, nil
}

func getDummyRestreamRouter() (*echo.Echo, error) {
	router := mock.DummyEcho()

	restream, err := getDummyRestreamHandler()
	if err != nil {
		return nil, err
	}

	router.GET("/", restream.GetAll)
	router.POST("/", restream.Add)
	router.GET("/:id", restream.Get)
	router.GET("/:id/config", restream.GetConfig)
	router.GET("/:id/report", restream.GetReport)
	router.GET("/:id/state", restream.GetState)
	router.PUT("/:id", restream.Update)
	router.DELETE("/:id", restream.Delete)
	router.PUT("/:id/command", restream.Command)
	router.GET("/:id/metadata", restream.GetProcessMetadata)
	router.GET("/:id/metadata/:key", restream.GetProcessMetadata)
	router.PUT("/:id/metadata/:key", restream.SetProcessMetadata)

	router.GET("/metadata", restream.GetMetadata)
	router.GET("/metadata/:key", restream.GetMetadata)
	router.PUT("/metadata/:key", restream.SetMetadata)

	router.GET("/report/process", restream.SearchReportHistory)

	return router, nil
}

func TestAddProcessMissingField(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcessMissingField.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
}

func TestAddProcessInvalidType(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcessInvalidType.json")

	mock.Request(t, http.StatusBadRequest, router, "POST", "/", data)
}

func TestAddProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	response := mock.Request(t, http.StatusOK, router, "POST", "/", data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)
}

func TestAddProcessWithMetadata(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := bytes.Buffer{}
	_, err = data.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	process := api.ProcessConfig{}
	err = json.Unmarshal(data.Bytes(), &process)
	require.NoError(t, err)

	process.Metadata = map[string]interface{}{
		"foo": "bar",
	}

	encoded, err := json.Marshal(&process)
	require.NoError(t, err)

	data.Reset()
	_, err = data.Write(encoded)
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "POST", "/", &data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	response = mock.Request(t, http.StatusOK, router, "GET", "/"+process.ID+"/metadata", nil)
	require.Equal(t, map[string]interface{}{
		"foo": "bar",
	}, response.Data)
}

func TestUpdateProcessInvalid(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	response := mock.Request(t, http.StatusOK, router, "POST", "/", data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	update := bytes.Buffer{}
	_, err = update.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	proc := api.ProcessConfig{}
	err = json.Unmarshal(update.Bytes(), &proc)
	require.NoError(t, err)

	// invalid address
	proc.Output[0].Address = ""

	encoded, err := json.Marshal(&proc)
	require.NoError(t, err)

	update.Reset()
	_, err = update.Write(encoded)
	require.NoError(t, err)

	mock.Request(t, http.StatusBadRequest, router, "PUT", "/"+proc.ID, &update)
	mock.Request(t, http.StatusOK, router, "GET", "/"+proc.ID, nil)
}

func TestUpdateReplaceProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	response := mock.Request(t, http.StatusOK, router, "POST", "/", data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	data = mock.Read(t, "./fixtures/addProcess.json")

	response = mock.Request(t, http.StatusOK, router, "PUT", "/test", data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)
}

func TestUpdateReplaceProcessWithMetadata(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := bytes.Buffer{}
	_, err = data.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	process := api.ProcessConfig{}
	err = json.Unmarshal(data.Bytes(), &process)
	require.NoError(t, err)

	process.Metadata = map[string]interface{}{
		"foo": "bar",
	}

	encoded, err := json.Marshal(&process)
	require.NoError(t, err)

	data.Reset()
	_, err = data.Write(encoded)
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "POST", "/", &data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	data.Reset()
	_, err = data.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	process = api.ProcessConfig{}
	err = json.Unmarshal(data.Bytes(), &process)
	require.NoError(t, err)

	process.Metadata = map[string]interface{}{
		"foo": "baz",
	}

	encoded, err = json.Marshal(&process)
	require.NoError(t, err)

	data.Reset()
	_, err = data.Write(encoded)
	require.NoError(t, err)

	response = mock.Request(t, http.StatusOK, router, "PUT", "/test", &data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	response = mock.Request(t, http.StatusOK, router, "GET", "/"+process.ID+"/metadata", nil)
	require.Equal(t, map[string]interface{}{
		"foo": "baz",
	}, response.Data)
}

func TestUpdateNewProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	response := mock.Request(t, http.StatusOK, router, "POST", "/", data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	update := bytes.Buffer{}
	_, err = update.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	proc := api.ProcessConfig{}
	err = json.Unmarshal(update.Bytes(), &proc)
	require.NoError(t, err)

	proc.ID = "test2"

	encoded, err := json.Marshal(&proc)
	require.NoError(t, err)

	update.Reset()
	_, err = update.Write(encoded)
	require.NoError(t, err)

	response = mock.Request(t, http.StatusOK, router, "PUT", "/test", &update)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	mock.Request(t, http.StatusNotFound, router, "GET", "/test", nil)
	mock.Request(t, http.StatusOK, router, "GET", "/test2", nil)
}

func TestUpdateNewProcessWithMetadata(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := bytes.Buffer{}
	_, err = data.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	process := api.ProcessConfig{}
	err = json.Unmarshal(data.Bytes(), &process)
	require.NoError(t, err)

	process.Metadata = map[string]interface{}{
		"foo": "bar",
	}

	encoded, err := json.Marshal(&process)
	require.NoError(t, err)

	data.Reset()
	_, err = data.Write(encoded)
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "POST", "/", &data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	data.Reset()
	_, err = data.ReadFrom(mock.Read(t, "./fixtures/addProcess.json"))
	require.NoError(t, err)

	process = api.ProcessConfig{}
	err = json.Unmarshal(data.Bytes(), &process)
	require.NoError(t, err)

	process.ID = "test2"
	process.Metadata = map[string]interface{}{
		"bar": "foo",
	}

	encoded, err = json.Marshal(&process)
	require.NoError(t, err)

	data.Reset()
	_, err = data.Write(encoded)
	require.NoError(t, err)

	response = mock.Request(t, http.StatusOK, router, "PUT", "/test", &data)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)

	mock.Request(t, http.StatusNotFound, router, "GET", "/test", nil)
	mock.Request(t, http.StatusOK, router, "GET", "/test2", nil)

	response = mock.Request(t, http.StatusOK, router, "GET", "/"+process.ID+"/metadata", nil)
	require.Equal(t, map[string]interface{}{
		"foo": "bar",
		"bar": "foo",
	}, response.Data)
}

func TestUpdateNonExistentProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusNotFound, router, "PUT", "/test", data)
}

func TestRemoveUnknownProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	mock.Request(t, http.StatusNotFound, router, "DELETE", "/foobar", nil)
}

func TestRemoveProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/removeProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	mock.Request(t, http.StatusOK, router, "DELETE", "/test", nil)
}

func TestAllProcesses(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	mock.Validate(t, &[]api.Process{}, response.Data)

	p := []api.Process{}
	err = json.Unmarshal(response.Raw, &p)
	require.NoError(t, err)

	require.Equal(t, 0, len(p))

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	response = mock.Request(t, http.StatusOK, router, "GET", "/", nil)

	mock.Validate(t, &[]api.Process{}, response.Data)

	p = []api.Process{}
	err = json.Unmarshal(response.Raw, &p)
	require.NoError(t, err)

	require.Equal(t, 1, len(p))
}

func TestProcess(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	mock.Request(t, http.StatusNotFound, router, "GET", "/test", nil)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	response := mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	mock.Validate(t, &api.Process{}, response.Data)
}

func TestProcessInfo(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	response := mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	mock.Validate(t, &api.Process{}, response.Data)
}

func TestProcessConfig(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	response := mock.Request(t, http.StatusOK, router, "GET", "/test/config", nil)

	mock.Validate(t, &api.ProcessConfig{}, response.Data)
}

func TestProcessState(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	response := mock.Request(t, http.StatusOK, router, "GET", "/test/state", nil)

	mock.Validate(t, &api.ProcessState{}, response.Data)
}

func TestProcessReportNotFound(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	mock.Request(t, http.StatusNotFound, router, "GET", "/test/report", nil)
}

func TestProcessReport(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)
	response := mock.Request(t, http.StatusOK, router, "GET", "/test/report", nil)

	mock.Validate(t, &api.ProcessReport{}, response.Data)
}

func TestProcessReportAt(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	command := mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	time.Sleep(2 * time.Second)

	command = mock.Read(t, "./fixtures/commandStop.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	command = mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	time.Sleep(2 * time.Second)

	command = mock.Read(t, "./fixtures/commandStop.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	response := mock.Request(t, http.StatusOK, router, "GET", "/test/report", nil)

	x := api.ProcessReport{}
	err = json.Unmarshal(response.Raw, &x)
	require.NoError(t, err)

	require.Equal(t, 2, len(x.History))

	created := x.History[0].CreatedAt
	exited := x.History[0].ExitedAt

	mock.Request(t, http.StatusOK, router, "GET", "/test/report?created_at="+strconv.FormatInt(created, 10), nil)
	mock.Request(t, http.StatusNotFound, router, "GET", "/test/report?created_at=1234", nil)

	mock.Request(t, http.StatusOK, router, "GET", "/test/report?exited_at="+strconv.FormatInt(exited, 10), nil)
	mock.Request(t, http.StatusNotFound, router, "GET", "/test/report?exited_at=1234", nil)

	exited = x.History[1].ExitedAt

	response = mock.Request(t, http.StatusOK, router, "GET", "/test/report?created_at="+strconv.FormatInt(created, 10)+"&exited_at="+strconv.FormatInt(exited, 10), nil)

	x = api.ProcessReport{}
	err = json.Unmarshal(response.Raw, &x)
	require.NoError(t, err)

	require.Equal(t, 2, len(x.History))
}

func TestSearchReportHistory(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	command := mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	time.Sleep(2 * time.Second)

	command = mock.Read(t, "./fixtures/commandStop.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	command = mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	time.Sleep(2 * time.Second)

	command = mock.Read(t, "./fixtures/commandStop.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", nil)

	response := mock.Request(t, http.StatusOK, router, "GET", "/test/report", nil)

	x := api.ProcessReport{}
	err = json.Unmarshal(response.Raw, &x)
	require.NoError(t, err)

	require.Equal(t, 2, len(x.History))

	time1 := x.History[0].ExitedAt
	time2 := x.History[1].ExitedAt

	response = mock.Request(t, http.StatusOK, router, "GET", "/report/process", nil)

	r := []api.ProcessReportSearchResult{}
	err = json.Unmarshal(response.Raw, &r)
	require.NoError(t, err)

	require.Equal(t, 2, len(r))

	response = mock.Request(t, http.StatusOK, router, "GET", "/report/process?state=failed", nil)

	r = []api.ProcessReportSearchResult{}
	err = json.Unmarshal(response.Raw, &r)
	require.NoError(t, err)

	require.Equal(t, 0, len(r))

	response = mock.Request(t, http.StatusOK, router, "GET", "/report/process?state=finished", nil)

	r = []api.ProcessReportSearchResult{}
	err = json.Unmarshal(response.Raw, &r)
	require.NoError(t, err)

	require.Equal(t, 2, len(r))

	response = mock.Request(t, http.StatusOK, router, "GET", "/report/process?from="+strconv.FormatInt(time1, 10), nil)

	r = []api.ProcessReportSearchResult{}
	err = json.Unmarshal(response.Raw, &r)
	require.NoError(t, err)

	require.Equal(t, 2, len(r))

	response = mock.Request(t, http.StatusOK, router, "GET", "/report/process?to="+strconv.FormatInt(time2, 10), nil)

	r = []api.ProcessReportSearchResult{}
	err = json.Unmarshal(response.Raw, &r)
	require.NoError(t, err)

	require.Equal(t, 1, len(r))

	response = mock.Request(t, http.StatusOK, router, "GET", "/report/process?from="+strconv.FormatInt(time1, 10)+"&to="+strconv.FormatInt(time2+1, 10), nil)

	r = []api.ProcessReportSearchResult{}
	err = json.Unmarshal(response.Raw, &r)
	require.NoError(t, err)

	require.Equal(t, 2, len(r))
}

func TestProcessCommandNotFound(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	command := mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusNotFound, router, "PUT", "/test/command", command)
}

func TestProcessCommandInvalid(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	command := mock.Read(t, "./fixtures/commandInvalid.json")
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/test/command", command)
}

func TestProcessCommand(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	command := mock.Read(t, "./fixtures/commandStart.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", data)

	command = mock.Read(t, "./fixtures/commandStop.json")
	mock.Request(t, http.StatusOK, router, "PUT", "/test/command", command)
	mock.Request(t, http.StatusOK, router, "GET", "/test", data)
}

func TestProcessMetadata(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	data := mock.Read(t, "./fixtures/addProcess.json")

	mock.Request(t, http.StatusOK, router, "POST", "/", data)

	response := mock.Request(t, http.StatusOK, router, "GET", "/test/metadata", nil)
	require.Equal(t, nil, response.Data)

	mock.Request(t, http.StatusNotFound, router, "GET", "/test/metadata/foobar", nil)

	data = bytes.NewReader([]byte("hello"))
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/test/metadata/foobar", data)

	data = bytes.NewReader([]byte(`"hello"`))
	mock.Request(t, http.StatusOK, router, "PUT", "/test/metadata/foobar", data)

	response = mock.Request(t, http.StatusOK, router, "GET", "/test/metadata/foobar", nil)

	x := ""
	err = json.Unmarshal(response.Raw, &x)
	require.NoError(t, err)

	require.Equal(t, "hello", x)

	data = bytes.NewReader([]byte(`null`))
	mock.Request(t, http.StatusOK, router, "PUT", "/test/metadata/foobar", data)

	mock.Request(t, http.StatusNotFound, router, "GET", "/test/metadata/foobar", nil)

	response = mock.Request(t, http.StatusOK, router, "GET", "/test/metadata", nil)
	require.Equal(t, nil, response.Data)
}

func TestMetadata(t *testing.T) {
	router, err := getDummyRestreamRouter()
	require.NoError(t, err)

	response := mock.Request(t, http.StatusOK, router, "GET", "/metadata", nil)
	require.Equal(t, nil, response.Data)

	mock.Request(t, http.StatusNotFound, router, "GET", "/metadata/foobar", nil)

	data := bytes.NewReader([]byte("hello"))
	mock.Request(t, http.StatusBadRequest, router, "PUT", "/metadata/foobar", data)

	data = bytes.NewReader([]byte(`"hello"`))
	mock.Request(t, http.StatusOK, router, "PUT", "/metadata/foobar", data)

	response = mock.Request(t, http.StatusOK, router, "GET", "/metadata/foobar", nil)

	x := ""
	err = json.Unmarshal(response.Raw, &x)
	require.NoError(t, err)

	require.Equal(t, "hello", x)

	data = bytes.NewReader([]byte(`null`))
	mock.Request(t, http.StatusOK, router, "PUT", "/metadata/foobar", data)

	mock.Request(t, http.StatusNotFound, router, "GET", "/metadata/foobar", nil)

	response = mock.Request(t, http.StatusOK, router, "GET", "/metadata", nil)
	require.Equal(t, nil, response.Data)
}

func BenchmarkAllProcesses(b *testing.B) {
	router, err := getDummyRestreamRouter()
	require.NoError(b, err)

	data := bytes.Buffer{}
	_, err = data.ReadFrom(mock.Read(b, "./fixtures/addProcess.json"))
	require.NoError(b, err)

	process := api.ProcessConfig{}
	err = json.Unmarshal(data.Bytes(), &process)
	require.NoError(b, err)

	for i := range 1000 {
		process.ID = "test_" + strconv.Itoa(i)

		encoded, err := json.Marshal(&process)
		require.NoError(b, err)

		data.Reset()
		_, err = data.Write(encoded)
		require.NoError(b, err)

		mock.Request(b, http.StatusOK, router, "POST", "/", &data)
	}

	for b.Loop() {
		response := mock.RequestEx(b, http.StatusOK, router, "GET", "/", nil, false)
		require.Equal(b, response.Code, 200)
	}
}
