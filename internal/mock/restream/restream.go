package restream

import (
	"fmt"

	"github.com/datarhei/core/v16/ffmpeg"
	"github.com/datarhei/core/v16/iam"
	iamidentity "github.com/datarhei/core/v16/iam/identity"
	"github.com/datarhei/core/v16/iam/policy"
	"github.com/datarhei/core/v16/internal/mock/resources"
	"github.com/datarhei/core/v16/internal/testhelper"
	"github.com/datarhei/core/v16/io/fs"
	"github.com/datarhei/core/v16/net"
	"github.com/datarhei/core/v16/restream"
	"github.com/datarhei/core/v16/restream/replace"
	"github.com/datarhei/core/v16/restream/rewrite"
	jsonstore "github.com/datarhei/core/v16/restream/store/json"
)

func New(portrange net.Portranger, validatorIn, validatorOut ffmpeg.Validator, replacer replace.Replacer) (restream.Restreamer, error) {
	binary, err := testhelper.BuildBinary("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("failed to build helper program: %w", err)
	}

	resources := resources.New()

	ffmpeg, err := ffmpeg.New(ffmpeg.Config{
		Binary:           binary,
		LogHistoryLength: 3,
		MaxLogLines:      100,
		Portrange:        portrange,
		ValidatorInput:   validatorIn,
		ValidatorOutput:  validatorOut,
		Resource:         resources,
	})
	if err != nil {
		return nil, err
	}

	memfs, err := fs.NewMemFilesystem(fs.MemConfig{})
	if err != nil {
		return nil, err
	}

	store, err := jsonstore.New(jsonstore.Config{
		Filesystem: memfs,
	})
	if err != nil {
		return nil, err
	}

	policyAdapter, err := policy.NewJSONAdapter(memfs, "./policy.json", nil)
	if err != nil {
		return nil, err
	}

	identityAdapter, err := iamidentity.NewJSONAdapter(memfs, "./users.json", nil)
	if err != nil {
		return nil, err
	}

	iam, err := iam.New(iam.Config{
		PolicyAdapter:   policyAdapter,
		IdentityAdapter: identityAdapter,
		Superuser: iamidentity.User{
			Name: "foobar",
		},
		JWTRealm:  "",
		JWTSecret: "",
		Logger:    nil,
	})
	if err != nil {
		return nil, err
	}

	iam.AddPolicy("$anon", "$none", []string{"process"}, "*", []string{"CREATE", "GET", "DELETE", "UPDATE", "COMMAND", "PROBE", "METADATA", "PLAYOUT"})

	rewriter, err := rewrite.New(rewrite.Config{
		IAM: iam,
	})
	if err != nil {
		return nil, err
	}

	rs, err := restream.New(restream.Config{
		Store:       store,
		FFmpeg:      ffmpeg,
		Replace:     replacer,
		Filesystems: []fs.Filesystem{memfs},
		Rewrite:     rewriter,
		Resources:   resources,
	})
	if err != nil {
		return nil, err
	}

	return rs, nil
}
