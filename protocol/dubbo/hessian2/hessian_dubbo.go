/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package hessian2

import (
	"bufio"
	"encoding/binary"
	"time"
)

import (
	"github.com/apache/dubbo-go-hessian2"

	perrors "github.com/pkg/errors"
)

// enum part
const (
	PackageError              = PackageType(0x01)
	PackageRequest            = PackageType(0x02)
	PackageResponse           = PackageType(0x04)
	PackageHeartbeat          = PackageType(0x08)
	PackageRequest_TwoWay     = PackageType(0x10)
	PackageResponse_Exception = PackageType(0x20)
	PackageType_BitSize       = 0x2f
)

// PackageType nolint
type PackageType int

// DubboHeader dubbo header
type DubboHeader struct {
	SerialID       byte
	Type           PackageType
	ID             int64
	BodyLen        int
	ResponseStatus byte
}

// Service defines service instance
type Service struct {
	Path      string
	Interface string
	Group     string
	Version   string
	Method    string
	Timeout   time.Duration // request timeout
}

// HessianCodec defines hessian codec
type HessianCodec struct {
	pkgType PackageType
	reader  *bufio.Reader
	bodyLen int
	stream  bool
}

// NewHessianCodec generate a new hessian codec instance
func NewHessianCodec(reader *bufio.Reader) *HessianCodec {
	return &HessianCodec{
		reader: reader,
	}
}

// NewHessianCodecCustom generate a new hessian codec instance
func NewHessianCodecCustom(pkgType PackageType, reader *bufio.Reader, bodyLen int) *HessianCodec {
	return &HessianCodec{
		pkgType: pkgType,
		reader:  reader,
		bodyLen: bodyLen,
	}
}

// NewStreamHessianCodecCustom generate a new hessian codec instance
func NewStreamHessianCodecCustom(pkgType PackageType, reader *bufio.Reader, bodyLen int) *HessianCodec {
	return &HessianCodec{
		pkgType: pkgType,
		reader:  reader,
		bodyLen: bodyLen,
		stream:  true,
	}
}

func (h *HessianCodec) Write(service Service, header DubboHeader, body any) ([]byte, error) {
	switch header.Type {
	case PackageHeartbeat:
		if header.ResponseStatus == Zero {
			return packRequest(service, header, body)
		}
		return packResponse(header, body)

	case PackageRequest, PackageRequest_TwoWay:
		return packRequest(service, header, body)

	case PackageResponse:
		return packResponse(header, body)

	default:
		return nil, perrors.Errorf("Unrecognized message type: %v", header.Type)
	}

	// unreachable return nil, nil
}

// ReadHeader uses hessian codec to read dubbo header
func (h *HessianCodec) ReadHeader(header *DubboHeader) error {
	var (
		err error
		n   int
		buf []byte
	)

	if h.stream {
		buf = make([]byte, HEADER_LENGTH)
		n, err = h.reader.Read(buf)
		if err != nil {
			return perrors.WithStack(err)
		}
		if n < HEADER_LENGTH {
			_, err = h.reader.Read(buf[n:])
			if err != nil { // this is impossible
				return perrors.WithStack(err)
			}
		}
	} else {
		if h.reader.Size() < HEADER_LENGTH {
			return ErrHeaderNotEnough
		}
		buf, err = h.reader.Peek(HEADER_LENGTH)
		if err != nil { // this is impossible
			return perrors.WithStack(err)
		}
		_, err = h.reader.Discard(HEADER_LENGTH)
		if err != nil { // this is impossible
			return perrors.WithStack(err)
		}
	}

	//// read header

	if buf[0] != MAGIC_HIGH && buf[1] != MAGIC_LOW {
		return ErrIllegalPackage
	}

	// Header{serialization id(5 bit), event, two way, req/response}
	if header.SerialID = buf[2] & SERIAL_MASK; header.SerialID == Zero {
		return perrors.Errorf("serialization ID:%v", header.SerialID)
	}

	flag := buf[2] & FLAG_EVENT
	if flag != Zero {
		header.Type |= PackageHeartbeat
	}
	flag = buf[2] & FLAG_REQUEST
	if flag != Zero {
		header.Type |= PackageRequest
		flag = buf[2] & FLAG_TWOWAY
		if flag != Zero {
			header.Type |= PackageRequest_TwoWay
		}
	} else {
		header.Type |= PackageResponse
		header.ResponseStatus = buf[3]
		if header.ResponseStatus != Response_OK {
			header.Type |= PackageResponse_Exception
		}
	}

	// Header{req id}
	header.ID = int64(binary.BigEndian.Uint64(buf[4:]))

	// Header{body len}
	header.BodyLen = int(binary.BigEndian.Uint32(buf[12:]))
	if header.BodyLen < 0 {
		return ErrIllegalPackage
	}

	h.pkgType = header.Type
	h.bodyLen = header.BodyLen

	if h.reader.Buffered() < h.bodyLen && !h.stream {
		return ErrBodyNotEnough
	}

	return perrors.WithStack(err)
}

// ReadBody uses hessian codec to read response body
func (h *HessianCodec) ReadBody(rspObj any) error {
	var (
		err error
		buf []byte
	)

	if h.stream {
		buf = make([]byte, h.bodyLen)
		readLen, n := 0, 0
		for readLen < h.bodyLen {
			n, err = h.reader.Read(buf[readLen:])
			if err != nil {
				return perrors.WithStack(err)
			}
			readLen += n
		}
	} else {
		if h.reader.Buffered() < h.bodyLen {
			return ErrBodyNotEnough
		}
		buf, err = h.reader.Peek(h.bodyLen)
		if err != nil {
			return perrors.WithStack(err)
		}
		_, err = h.reader.Discard(h.bodyLen)
		if err != nil { // this is impossible
			return perrors.WithStack(err)
		}
	}

	switch h.pkgType & PackageType_BitSize {
	case PackageResponse | PackageHeartbeat | PackageResponse_Exception, PackageResponse | PackageResponse_Exception:
		decoder := hessian.NewDecoder(buf[:])
		exception, exceptionErr := decoder.Decode()
		if exceptionErr != nil {
			return perrors.WithStack(exceptionErr)
		}
		rsp, ok := rspObj.(*DubboResponse)
		if !ok {
			return perrors.Errorf("java exception:%s", exception.(string))
		}
		rsp.Exception = perrors.Errorf("java exception:%s", exception.(string))
		return nil
	case PackageRequest | PackageHeartbeat, PackageResponse | PackageHeartbeat:
	case PackageRequest:
		if rspObj != nil {
			if err = unpackRequestBody(hessian.NewDecoder(buf[:]), rspObj); err != nil {
				return perrors.WithStack(err)
			}
		}
	case PackageResponse:
		if rspObj != nil {
			if err = unpackResponseBody(hessian.NewDecoder(buf[:]), rspObj); err != nil {
				return perrors.WithStack(err)
			}
		}
	}

	return nil
}

// ignore body, but only read attachments
func (h *HessianCodec) ReadAttachments() (map[string]any, error) {
	var (
		err error
		buf []byte
	)

	if h.stream {
		buf = make([]byte, h.bodyLen)
		readLen, n := 0, 0
		for readLen < h.bodyLen {
			n, err = h.reader.Read(buf[readLen:])
			if err != nil {
				return nil, perrors.WithStack(err)
			}
			readLen += n
		}
	} else {
		if h.reader.Buffered() < h.bodyLen {
			return nil, ErrBodyNotEnough
		}
		buf, err = h.reader.Peek(h.bodyLen)
		if err != nil {
			return nil, perrors.WithStack(err)
		}
		_, err = h.reader.Discard(h.bodyLen)
		if err != nil { // this is impossible
			return nil, perrors.WithStack(err)
		}
	}

	switch h.pkgType & PackageType_BitSize {
	case PackageRequest:
		rspObj := make([]any, 7)
		if err = unpackRequestBody(hessian.NewDecoderWithSkip(buf[:]), rspObj); err != nil {
			return nil, perrors.WithStack(err)
		}
		return rspObj[6].(map[string]any), nil
	case PackageResponse:
		rspObj := &DubboResponse{}
		if err = unpackResponseBody(hessian.NewDecoderWithSkip(buf[:]), rspObj); err != nil {
			return nil, perrors.WithStack(err)
		}
		return rspObj.Attachments, nil
	}

	return nil, nil
}
