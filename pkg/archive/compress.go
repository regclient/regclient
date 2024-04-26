package archive

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"io"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
)

// CompressType identifies the detected compression type
type CompressType int

const (
	// CompressNone detected no compression
	CompressNone CompressType = iota
	// CompressBzip2 compression
	CompressBzip2
	// CompressGzip compression
	CompressGzip
	// CompressXz compression
	CompressXz
	// CompressZstd compression
	CompressZstd
)

// compressHeaders are used to detect the compression type
var compressHeaders = map[CompressType][]byte{
	CompressBzip2: []byte("\x42\x5A\x68"),
	CompressGzip:  []byte("\x1F\x8B\x08"),
	CompressXz:    []byte("\xFD\x37\x7A\x58\x5A\x00"),
	CompressZstd:  []byte("\x28\xB5\x2F\xFD"),
}

func Compress(r io.Reader, oComp CompressType) (io.Reader, error) {
	br := bufio.NewReader(r)

	var rComp CompressType

	// Peek length of the magic header for the given compression into the stream
	head, err := br.Peek(len(compressHeaders[oComp]))
	if err != nil {
		if err == io.EOF {
			// Not enough bytes to peek, that usually means the stream is shorter than
			// length of the magic bytes header which is a sign of uncompressed stream.
			rComp = CompressNone
		} else {
			return br, err
		}
	} else {
		// Detect the compression type
		rComp = DetectCompression(head)
	}

	// If the detected compression matches the requested compression, then return.
	if rComp == oComp {
		return br, nil
	}

	var dbr io.Reader = br
	// If the stream is already compressed, then decompress it first
	if oComp != CompressNone {
		dbr, err = Decompress(br)
		if err != nil {
			println("here")
			return nil, err
		}
	}

	switch oComp {
	case CompressGzip:
		return compressGzip(dbr)
	case CompressBzip2:
		// https://github.com/golang/go/issues/4828
		return nil, ErrNotImplemented
	case CompressXz:
		return compressXz(br)
	case CompressZstd:
		return compressZstd(br)

	}
	// No other types currently supported
	return nil, ErrUnknownType
}

func compressXz(src io.Reader) (io.Reader, error) {
	pipeR, pipeW := io.Pipe()
	go func() {
		defer pipeW.Close()
		xzW, _ := xz.NewWriter(pipeW)
		defer xzW.Close()
		_, _ = io.Copy(xzW, src)
	}()
	return pipeR, nil
}

func compressZstd(src io.Reader) (io.Reader, error) {
	pipeR, pipeW := io.Pipe()
	go func() {
		defer pipeW.Close()
		zstdW, _ := zstd.ZipCompressor()(pipeW)
		defer zstdW.Close()
		_, _ = io.Copy(zstdW, src)
	}()
	return pipeR, nil
}

func compressGzip(src io.Reader) (io.Reader, error) {
	pipeR, pipeW := io.Pipe()
	go func() {
		defer pipeW.Close()
		gzipW := gzip.NewWriter(pipeW)
		defer gzipW.Close()
		_, _ = io.Copy(gzipW, src)
	}()
	return pipeR, nil
}

// Decompress extracts gzip and bzip streams
func Decompress(r io.Reader) (io.Reader, error) {
	// create bufio to peak on first few bytes
	br := bufio.NewReader(r)
	var comp CompressType
	head, err := br.Peek(10)
	if err != nil {
		if err == io.EOF {
			// Not enough bytes to peek, that usually means the stream is shorter than
			// length of the magic bytes header which is a sign of uncompressed stream.
			comp = CompressNone
		} else {
			return br, err
		}
	} else {
		comp = DetectCompression(head)
	}

	// compare peaked data against known compression types
	switch comp {
	case CompressBzip2:
		return bzip2.NewReader(br), nil
	case CompressGzip:
		return gzip.NewReader(br)
	case CompressXz:
		return xz.NewReader(br)
	case CompressZstd:
		return zstd.ZipDecompressor()(br), nil
	default:
		return br, nil
	}
}

// DetectCompression identifies the compression type based on the first few bytes
func DetectCompression(head []byte) CompressType {
	for c, b := range compressHeaders {
		if bytes.HasPrefix(head, b) {
			return c
		}
	}
	return CompressNone
}

func (ct CompressType) String() string {
	switch ct {
	case CompressNone:
		return "none"
	case CompressBzip2:
		return "bzip2"
	case CompressGzip:
		return "gzip"
	case CompressXz:
		return "xz"
	case CompressZstd:
		return "zstd"
	}
	return "unknown"
}
