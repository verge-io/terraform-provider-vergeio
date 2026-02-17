package vergeos

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// DefaultChunkSize is the default chunk size for file uploads (256KB).
// This matches the chunk size used by verge-cli and PSVergeOS.
const DefaultChunkSize = 262144

// FileService handles file read operations.
type FileService struct {
	client *Client
}

// List returns all files, with optional filtering and pagination.
func (s *FileService) List(ctx context.Context, opts ...ListOption) ([]File, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = fileListFields
	}

	params := options.toQueryParams()

	var files []File
	if err := s.client.get(ctx, "/files", params, &files); err != nil {
		return nil, err
	}

	return files, nil
}

// Get returns a single file by ID.
func (s *FileService) Get(ctx context.Context, id int) (*File, error) {
	params := url.Values{}
	params.Set("fields", fileListFields)

	var file File
	endpoint := fmt.Sprintf("/files/%d", id)
	if err := s.client.get(ctx, endpoint, params, &file); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "File", ID: id}
		}
		return nil, err
	}

	return &file, nil
}

// GetByName returns a file by name.
func (s *FileService) GetByName(ctx context.Context, name string) (*File, error) {
	files, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(files) == 0 {
		return nil, &NotFoundError{Resource: "File", ID: name}
	}

	return &files[0], nil
}

// ListISOs returns all ISO files.
func (s *FileService) ListISOs(ctx context.Context, opts ...ListOption) ([]File, error) {
	opts = append([]ListOption{WithFilter("type eq 'iso'")}, opts...)
	return s.List(ctx, opts...)
}

// Create creates a new file entry in VergeOS.
// This creates the metadata entry; use Upload() or UploadFrom() to upload content.
// If URL is provided in the request, VergeOS will import the file from that URL.
func (s *FileService) Create(ctx context.Context, req *FileCreateRequest) (*File, error) {
	var resp apiResponse
	if err := s.client.post(ctx, "/files", req, &resp); err != nil {
		return nil, err
	}

	// Get the created file ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a file's metadata.
func (s *FileService) Update(ctx context.Context, id int, req *FileUpdateRequest) (*File, error) {
	endpoint := fmt.Sprintf("/files/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete removes a file from VergeOS.
// Files that are referenced by VM drives cannot be deleted until the reference is removed.
func (s *FileService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/files/%d", id)
	return s.client.delete(ctx, endpoint)
}

// Download downloads a file from VergeOS and returns an io.ReadCloser.
// The caller is responsible for closing the reader when done.
// Use the File.Filesize field to know the expected size.
func (s *FileService) Download(ctx context.Context, id int) (io.ReadCloser, *File, error) {
	// First get file info
	file, err := s.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	// Build download URL with download=1 parameter
	encodedName := url.QueryEscape(file.Name)
	endpoint := fmt.Sprintf("/files/%d", id)
	params := url.Values{}
	params.Set("download", "1")
	params.Set("asname", encodedName)

	resp, err := s.client.request(ctx, http.MethodGet, endpoint, nil, params)
	if err != nil {
		return nil, nil, err
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, &APIError{
			StatusCode: resp.StatusCode,
			Endpoint:   endpoint,
			Message:    fmt.Sprintf("download failed with status %d", resp.StatusCode),
		}
	}

	return resp.Body, file, nil
}

// DownloadToFile downloads a file from VergeOS and saves it to the specified path.
// If destPath is a directory, the file will be saved with its original name.
// Returns the full path to the saved file.
func (s *FileService) DownloadToFile(ctx context.Context, id int, destPath string) (string, error) {
	reader, file, err := s.Download(ctx, id)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// Determine output path
	outputPath := destPath
	if info, err := os.Stat(destPath); err == nil && info.IsDir() {
		outputPath = filepath.Join(destPath, file.Name)
	}

	// Create output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("vergeos: failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Copy data
	_, err = io.Copy(outFile, reader)
	if err != nil {
		return "", fmt.Errorf("vergeos: failed to write file: %w", err)
	}

	return outputPath, nil
}

// Upload uploads a file to VergeOS from an io.Reader.
// The file entry must be created first with Create().
// The size parameter must match the allocated_bytes used when creating the file entry.
// Returns the updated File after upload completes.
func (s *FileService) Upload(ctx context.Context, id int, reader io.Reader, size int64) (*File, error) {
	return s.UploadWithChunkSize(ctx, id, reader, size, DefaultChunkSize)
}

// UploadWithChunkSize uploads a file with a custom chunk size.
// The file entry must be created first with Create().
func (s *FileService) UploadWithChunkSize(ctx context.Context, id int, reader io.Reader, size int64, chunkSize int) (*File, error) {
	var offset int64

	buffer := make([]byte, chunkSize)

	for offset < size {
		// Read a chunk
		bytesToRead := int64(chunkSize)
		if remaining := size - offset; remaining < bytesToRead {
			bytesToRead = remaining
		}

		n, err := io.ReadFull(reader, buffer[:bytesToRead])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, fmt.Errorf("vergeos: failed to read chunk at offset %d: %w", offset, err)
		}

		if n == 0 {
			break
		}

		// Upload chunk
		if err := s.uploadChunk(ctx, id, buffer[:n], offset); err != nil {
			return nil, fmt.Errorf("vergeos: failed to upload chunk at offset %d: %w", offset, err)
		}

		offset += int64(n)
	}

	// Return updated file info
	return s.Get(ctx, id)
}

// UploadFromFile uploads a local file to VergeOS.
// This is a convenience method that creates the file entry and uploads the content.
// Returns the created File after upload completes.
func (s *FileService) UploadFromFile(ctx context.Context, localPath string, req *FileCreateRequest) (*File, error) {
	// Open local file
	file, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("vergeos: failed to open file: %w", err)
	}
	defer file.Close()

	// Get file size
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("vergeos: failed to stat file: %w", err)
	}
	fileSize := info.Size()

	// Set defaults from local file if not provided
	if req.Name == "" {
		req.Name = filepath.Base(localPath)
	}
	if req.AllocatedBytes == "" {
		req.AllocatedBytes = fmt.Sprintf("%d", fileSize)
	}

	// Create file entry
	createdFile, err := s.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vergeos: failed to create file entry: %w", err)
	}

	// Upload content
	uploadedFile, err := s.Upload(ctx, int(createdFile.ID), file, fileSize)
	if err != nil {
		// Try to clean up on failure
		_ = s.Delete(ctx, int(createdFile.ID))
		return nil, err
	}

	return uploadedFile, nil
}

// uploadChunk uploads a single chunk of data to a file.
func (s *FileService) uploadChunk(ctx context.Context, id int, data []byte, offset int64) error {
	// Build URL with filepos parameter
	u := fmt.Sprintf("%s%s/files/%d?filepos=%d", s.client.baseURL, apiBasePath, id, offset)

	// Create request with binary body
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, nil)
	if err != nil {
		return fmt.Errorf("vergeos: failed to create request: %w", err)
	}

	// Set authentication header
	if s.client.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.client.apiKey)
	} else {
		req.SetBasicAuth(s.client.username, s.client.password)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("User-Agent", s.client.userAgent)

	// Set body
	req.Body = io.NopCloser(newBytesReader(data))
	req.ContentLength = int64(len(data))

	// Execute request
	resp, err := s.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vergeos: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Endpoint:   fmt.Sprintf("/files/%d?filepos=%d", id, offset),
			Message:    string(body),
		}
	}

	return nil
}

// bytesReader wraps a byte slice to implement io.Reader.
type bytesReader struct {
	data   []byte
	offset int
}

func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data}
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
