package compressor

import (
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/milvus-io/milvus/internal/util/mock"
	"github.com/stretchr/testify/assert"
)

func TestZstdCompress(t *testing.T) {
	data := "hello zstd algorithm!"
	compressed := new(bytes.Buffer)
	origin := new(bytes.Buffer)

	enc, err := NewZstdCompressor(compressed)
	assert.NoError(t, err)
	testCompress(t, data, enc, compressed, origin)

	// Reuse test
	compressed.Reset()
	origin.Reset()

	enc.ResetWriter(compressed)

	testCompress(t, data+": reuse", enc, compressed, origin)
}

func testCompress(t *testing.T, data string, enc Compressor, compressed, origin *bytes.Buffer) {
	err := enc.Compress(strings.NewReader(data))
	assert.NoError(t, err)
	err = enc.Close()
	assert.NoError(t, err)

	// Close() method should satisfy idempotence
	err = enc.Close()
	assert.NoError(t, err)

	dec, err := NewZstdDecompressor(compressed)
	assert.NoError(t, err)
	err = dec.Decompress(origin)
	assert.NoError(t, err)

	assert.Equal(t, data, origin.String())

	// Mock error reader/writer
	errReader := &mock.ErrReader{Err: io.ErrUnexpectedEOF}
	errWriter := &mock.ErrWriter{Err: io.ErrShortWrite}

	err = enc.Compress(errReader)
	assert.ErrorIs(t, err, errReader.Err)

	dec.ResetReader(bytes.NewReader(compressed.Bytes()))
	err = dec.Decompress(errWriter)
	assert.ErrorIs(t, err, errWriter.Err)

	// Use closed decompressor
	dec.ResetReader(bytes.NewReader(compressed.Bytes()))
	dec.Close()
	err = dec.Decompress(origin)
	assert.Error(t, err)
}

func TestGlobalMethods(t *testing.T) {
	data := "hello zstd algorithm!"
	compressed := new(bytes.Buffer)
	origin := new(bytes.Buffer)

	err := ZstdCompress(strings.NewReader(data), compressed)
	assert.NoError(t, err)

	err = ZstdDecompress(compressed, origin)
	assert.NoError(t, err)

	assert.Equal(t, data, origin.String())

	// Mock error reader/writer
	errReader := &mock.ErrReader{Err: io.ErrUnexpectedEOF}
	errWriter := &mock.ErrWriter{Err: io.ErrShortWrite}

	compressedBytes := compressed.Bytes()
	compressed = bytes.NewBuffer(compressedBytes) // The old compressed buffer is closed
	err = ZstdCompress(errReader, compressed)
	assert.ErrorIs(t, err, errReader.Err)

	assert.Positive(t, len(compressedBytes))
	reader := bytes.NewReader(compressedBytes)
	err = ZstdDecompress(reader, errWriter)
	assert.ErrorIs(t, err, errWriter.Err)

	// Incorrect option
	err = ZstdCompress(strings.NewReader(data), compressed, zstd.WithWindowSize(3))
	assert.Error(t, err)

	err = ZstdDecompress(compressed, origin, zstd.WithDecoderConcurrency(0))
	assert.Error(t, err)
}

func TestCurrencyGlobalMethods(t *testing.T) {
	prefix := "Test Currency Global Methods"

	currency := runtime.GOMAXPROCS(0) * 2
	if currency < 6 {
		currency = 6
	}

	wg := sync.WaitGroup{}
	wg.Add(currency)
	for i := 0; i < currency; i++ {
		go func(idx int) {
			defer wg.Done()

			buf := new(bytes.Buffer)
			origin := new(bytes.Buffer)

			data := prefix + fmt.Sprintf(": %d-th goroutine", idx)

			err := ZstdCompress(strings.NewReader(data), buf, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(idx)))
			assert.NoError(t, err)

			err = ZstdDecompress(buf, origin)
			assert.NoError(t, err)

			assert.Equal(t, data, origin.String())
		}(i)
	}
	wg.Wait()
}
