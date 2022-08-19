package fs

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3FSConfig struct {
	Base            string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
	Bucket          string
	UseSSL          bool
}

type s3fs struct {
	base string

	endpoint        string
	accessKeyID     string
	secretAccessKey string
	region          string
	bucket          string
	useSSL          bool

	client *minio.Client
}

func NewS3FS(config S3FSConfig) (Filesystem, error) {
	fs := &s3fs{
		base:            config.Base,
		endpoint:        config.Endpoint,
		accessKeyID:     config.AccessKeyID,
		secretAccessKey: config.SecretAccessKey,
		region:          config.Region,
		bucket:          config.Bucket,
		useSSL:          config.UseSSL,
	}

	client, err := minio.New(fs.endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(fs.accessKeyID, fs.secretAccessKey, ""),
		Region: fs.region,
		Secure: fs.useSSL,
	})

	if err != nil {
		return nil, err
	}

	fs.client = client

	return fs, nil
}

func (fs *s3fs) Base() string {
	return fs.base
}

func (fs *s3fs) Rebase(base string) error {
	fs.base = base

	return nil
}

func (fs *s3fs) Size() (int64, int64) {
	return -1, -1
}

func (fs *s3fs) Resize(size int64) {}

func (fs *s3fs) Files() int64 {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := fs.client.ListObjects(ctx, fs.bucket, minio.ListObjectsOptions{
		WithVersions: false,
		WithMetadata: false,
		Prefix:       "",
		Recursive:    true,
		MaxKeys:      0,
		StartAfter:   "",
		UseV1:        false,
	})

	nfiles := int64(0)

	for range ch {
		nfiles++
	}

	return nfiles
}

func (fs *s3fs) Symlink(oldname, newname string) error {
	return fmt.Errorf("not implemented")
}

func (fs *s3fs) Open(path string) File {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	object, err := fs.client.GetObject(ctx, fs.bucket, path, minio.GetObjectOptions{})

	if err != nil {
		return nil
	}

	stat, err := object.Stat()
	if err != nil {
		return nil
	}

	file := &s3File{
		data:         object,
		name:         stat.Key,
		size:         stat.Size,
		lastModified: stat.LastModified,
	}

	return file
}

func (fs *s3fs) Store(path string, r io.Reader) (int64, bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	overwrite := false

	_, err := fs.client.StatObject(ctx, fs.bucket, path, minio.StatObjectOptions{})
	if err == nil {
		overwrite = true
	}

	info, err := fs.client.PutObject(ctx, fs.bucket, path, r, -1, minio.PutObjectOptions{
		UserMetadata:            map[string]string{},
		UserTags:                map[string]string{},
		Progress:                nil,
		ContentType:             "",
		ContentEncoding:         "",
		ContentDisposition:      "",
		ContentLanguage:         "",
		CacheControl:            "",
		Mode:                    "",
		RetainUntilDate:         time.Time{},
		ServerSideEncryption:    nil,
		NumThreads:              0,
		StorageClass:            "",
		WebsiteRedirectLocation: "",
		PartSize:                0,
		LegalHold:               "",
		SendContentMd5:          false,
		DisableContentSha256:    false,
		DisableMultipart:        false,
		Internal:                minio.AdvancedPutOptions{},
	})
	if err != nil {
		return -1, false, err
	}

	return info.Size, overwrite, nil
}

func (fs *s3fs) Delete(path string) int64 {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stat, err := fs.client.StatObject(ctx, fs.bucket, path, minio.StatObjectOptions{})
	if err != nil {
		return -1
	}

	err = fs.client.RemoveObject(ctx, fs.bucket, path, minio.RemoveObjectOptions{
		GovernanceBypass: true,
	})
	if err != nil {
		return -1
	}

	return stat.Size
}

func (fs *s3fs) DeleteAll() int64 {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	totalSize := int64(0)

	objectsCh := make(chan minio.ObjectInfo)

	// Send object names that are needed to be removed to objectsCh
	go func() {
		defer close(objectsCh)

		for object := range fs.client.ListObjects(ctx, fs.bucket, minio.ListObjectsOptions{
			Recursive: true,
		}) {
			if object.Err != nil {
				//log.Fatalln(object.Err)
				continue
			}
			totalSize += object.Size
			objectsCh <- object
		}
	}()

	for err := range fs.client.RemoveObjects(context.Background(), fs.bucket, objectsCh, minio.RemoveObjectsOptions{
		GovernanceBypass: true,
	}) {
		fmt.Println("Error detected during deletion: ", err)
	}

	return totalSize
}

func (fs *s3fs) List(pattern string) []FileInfo {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := fs.client.ListObjects(ctx, fs.bucket, minio.ListObjectsOptions{
		WithVersions: false,
		WithMetadata: false,
		Prefix:       "",
		Recursive:    true,
		MaxKeys:      0,
		StartAfter:   "",
		UseV1:        false,
	})

	files := []FileInfo{}

	for object := range ch {
		if object.Err != nil {
			return nil
		}

		f := &s3FileInfo{
			name:         object.Key,
			size:         object.Size,
			lastModified: object.LastModified,
		}

		files = append(files, f)
	}

	return files
}

type s3FileInfo struct {
	name         string
	size         int64
	lastModified time.Time
}

func (f *s3FileInfo) Name() string {
	return f.name
}

func (f *s3FileInfo) Size() int64 {
	return f.size
}

func (f *s3FileInfo) ModTime() time.Time {
	return f.lastModified
}

func (f *s3FileInfo) IsLink() (string, bool) {
	return "", false
}

func (f *s3FileInfo) IsDir() bool {
	return false
}

type s3File struct {
	data         io.ReadCloser
	name         string
	size         int64
	lastModified time.Time
}

func (f *s3File) Read(p []byte) (int, error) {
	return f.data.Read(p)
}

func (f *s3File) Close() error {
	return f.data.Close()
}

func (f *s3File) Name() string {
	return f.name
}

func (f *s3File) Stat() (FileInfo, error) {
	return &s3FileInfo{
		name:         f.name,
		size:         f.size,
		lastModified: f.lastModified,
	}, nil
}
