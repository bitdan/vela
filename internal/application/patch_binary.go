package application

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"strings"
)

// MaxImportedBinaryBytes bounds both inflated literals and delta results,
// independently of the compressed patch's existing input-size limit.
const MaxImportedBinaryBytes = 2 << 20
const gitBinaryAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz!#$%&()*+-;<=>?@^_`{|}~"

type binaryFragment struct {
	size, source int
	delta        bool
}

func decodeGitBinaryLine(line string) ([]byte, error) {
	if len(line) < 2 {
		return nil, errors.New("invalid binary patch data line")
	}
	n := 0
	switch {
	case line[0] >= 'A' && line[0] <= 'Z':
		n = int(line[0]-'A') + 1
	case line[0] >= 'a' && line[0] <= 'z':
		n = int(line[0]-'a') + 27
	default:
		return nil, errors.New("invalid binary patch line length")
	}
	if len(line) != 1+((n+3)/4)*5 {
		return nil, errors.New("binary patch line length does not match payload")
	}
	var lookup [256]uint8
	for i := range gitBinaryAlphabet {
		lookup[gitBinaryAlphabet[i]] = uint8(i + 1)
	}
	out := make([]byte, ((n+3)/4)*4)
	for i := 1; i < len(line); i += 5 {
		var value uint64
		for j := 0; j < 5; j++ {
			digit := lookup[line[i+j]]
			if digit == 0 {
				return nil, errors.New("invalid Git binary base85 alphabet")
			}
			value = value*85 + uint64(digit-1)
		}
		if value > 0xffffffff {
			return nil, errors.New("binary patch base85 value overflows")
		}
		binary.BigEndian.PutUint32(out[((i-1)/5)*4:], uint32(value))
	}
	return out[:n], nil
}

func binaryDeltaSize(data []byte, offset *int) (int, error) {
	var value uint64
	for shift := uint(0); shift < 63; shift += 7 {
		if *offset >= len(data) {
			break
		}
		b := data[*offset]
		*offset = *offset + 1
		value |= uint64(b&0x7f) << shift
		if value > MaxImportedBinaryBytes {
			return 0, errors.New("binary delta source/result exceeds 2 MiB")
		}
		if b&0x80 == 0 {
			return int(value), nil
		}
	}
	return 0, errors.New("invalid binary delta size")
}

// validateImportedBinary validates both directions and bounds decompression.
// Git remains authoritative for delta instruction execution and object hashes.
func validateImportedBinary(payload string) (oldSize, newSize int, err error) {
	bad := func() (int, int, error) {
		return 0, 0, errors.New("malformed, incomplete or oversized Git binary patch")
	}
	lines := strings.Split(payload, "\n")
	if len(lines) < 5 || lines[0] != "GIT binary patch" {
		return bad()
	}
	i := 1
	var fragments [2]binaryFragment
	for direction := range fragments {
		if i >= len(lines) {
			return bad()
		}
		fields := strings.Fields(lines[i])
		if len(fields) != 2 || fields[0] != "literal" && fields[0] != "delta" {
			return bad()
		}
		size, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil || size < 0 || size > MaxImportedBinaryBytes {
			return bad()
		}
		i++
		var compressed []byte
		for i < len(lines) && lines[i] != "" {
			chunk, decodeErr := decodeGitBinaryLine(lines[i])
			if decodeErr != nil {
				return 0, 0, decodeErr
			}
			compressed = append(compressed, chunk...)
			i++
		}
		if len(compressed) == 0 || i >= len(lines) {
			return bad()
		}
		i++
		reader := bytes.NewReader(compressed)
		inflater, zErr := zlib.NewReader(reader)
		if zErr != nil {
			return 0, 0, zErr
		}
		decoded, readErr := io.ReadAll(io.LimitReader(inflater, MaxImportedBinaryBytes+1))
		closeErr := inflater.Close()
		if readErr != nil || closeErr != nil || len(decoded) != size || reader.Len() != 0 {
			return bad()
		}
		fragment := binaryFragment{size: size}
		if fields[0] == "delta" {
			fragment.delta = true
			offset := 0
			fragment.source, err = binaryDeltaSize(decoded, &offset)
			if err != nil {
				return 0, 0, err
			}
			fragment.size, err = binaryDeltaSize(decoded, &offset)
			if err != nil {
				return 0, 0, err
			}
		}
		fragments[direction] = fragment
	}
	for ; i < len(lines); i++ {
		if lines[i] != "" {
			return bad()
		}
	}
	forward, reverse := fragments[0], fragments[1]
	if forward.delta && forward.source != reverse.size || reverse.delta && reverse.source != forward.size {
		return bad()
	}
	return reverse.size, forward.size, nil
}
