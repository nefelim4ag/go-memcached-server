package memcachedprotocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"nefelim4ag/go-memcached-server/memstore"
	"unsafe"

	"log/slog"
)

// opcodes
type OpcodeType byte

const (
	Get                        OpcodeType = 0x00
	Set                        OpcodeType = 0x01
	Add                        OpcodeType = 0x02
	Replace                    OpcodeType = 0x03
	Delete                     OpcodeType = 0x04
	Increment                  OpcodeType = 0x05
	Decrement                  OpcodeType = 0x06
	Quit                       OpcodeType = 0x07
	Flush                      OpcodeType = 0x08
	GetQ                       OpcodeType = 0x09
	NoOp                       OpcodeType = 0x0a
	Version                    OpcodeType = 0x0b
	GetK                       OpcodeType = 0x0c
	GetKQ                      OpcodeType = 0x0d
	Append                     OpcodeType = 0x0e
	Prepend                    OpcodeType = 0x0f
	Stat                       OpcodeType = 0x10
	SetQ                       OpcodeType = 0x11
	AddQ                       OpcodeType = 0x12
	ReplaceQ                   OpcodeType = 0x13
	DeleteQ                    OpcodeType = 0x14
	IncrementQ                 OpcodeType = 0x15
	DecrementQ                 OpcodeType = 0x16
	QuitQ                      OpcodeType = 0x17
	FlushQ                     OpcodeType = 0x18
	AppendQ                    OpcodeType = 0x19
	PrependQ                   OpcodeType = 0x1a
	VerbosityUnstable          OpcodeType = 0x1b
	TouchUnstable              OpcodeType = 0x1c
	GATUnstable                OpcodeType = 0x1d
	GATQUnstable               OpcodeType = 0x1e
	SASLlistmechs              OpcodeType = 0x20
	SASLAuth                   OpcodeType = 0x21
	SASLStep                   OpcodeType = 0x22
	RGet                       OpcodeType = 0x30
	RSet                       OpcodeType = 0x31
	RSetQ                      OpcodeType = 0x32
	RAppend                    OpcodeType = 0x33
	RAppendQ                   OpcodeType = 0x34
	RPrepend                   OpcodeType = 0x35
	RPrependQ                  OpcodeType = 0x36
	RDelete                    OpcodeType = 0x37
	RDeleteQ                   OpcodeType = 0x38
	RIncr                      OpcodeType = 0x39
	RIncrQ                     OpcodeType = 0x3a
	RDecr                      OpcodeType = 0x3b
	RDecrQ                     OpcodeType = 0x3c
	SetVBucketUnstable         OpcodeType = 0x3d
	GetVBucketUnstable         OpcodeType = 0x3e
	DelVBucketUnstable         OpcodeType = 0x3f
	TAPConnectUnstable         OpcodeType = 0x40
	TAPMutationUnstable        OpcodeType = 0x41
	TAPDeleteUnstable          OpcodeType = 0x42
	TAPFlushUnstable           OpcodeType = 0x43
	TAPOpaqueUnstable          OpcodeType = 0x44
	TAPVBucketSetUnstable      OpcodeType = 0x45
	TAPCheckpointStartUnstable OpcodeType = 0x46
	TAPCheckpointEndUnstable   OpcodeType = 0x47
)

// Response status
type ResponseStatus uint16

const (
	NoErr      ResponseStatus = 0x0000 // No error
	NEnt       ResponseStatus = 0x0001 // Key not found
	Exist      ResponseStatus = 0x0002 // Key exists
	TooLarg    ResponseStatus = 0x0003 // Value too large
	InvArg     ResponseStatus = 0x0004 // Invalid arguments
	ItemNoStor ResponseStatus = 0x0005 // Item not stored
	EType      ResponseStatus = 0x0006 // Incr/Decr on non-numeric value.
	// 0x0007	The vbucket belongs to another server
	// 0x0008	Authentication error
	// 0x0009	Authentication continue
	EUnknown ResponseStatus = 0x0081 // Unknown command
	EOOM     ResponseStatus = 0x0082 //	Out of memory
	ENSupp   ResponseStatus = 0x0083 // Not supported
	EInter   ResponseStatus = 0x0084 // Internal error
	EBusy    ResponseStatus = 0x0085 // Busy
	ETmpF    ResponseStatus = 0x0086 // Temporary failure
)

// Magic
type Magic byte

const (
	RequestMagic  Magic = 0x80
	ResponseMagic Magic = 0x81
)

type RequestHeader struct {
	magic     Magic
	opcode    OpcodeType
	keyLen    uint16
	extrasLen uint8
	dataType  uint8
	vBucketId uint16
	totalBody uint32
	opaque    [4]byte
	cas       uint64
}

type ResponseHeader struct {
	magic     Magic
	opcode    OpcodeType
	keyLen    uint16
	extrasLen uint8
	dataType  uint8
	status    ResponseStatus
	totalBody uint32
	opaque    [4]byte
	cas       uint64
}

func (ctx *Processor) CommandBinary() error {
	err := ctx.ReadRequest()
	if err != nil {
		return err
	}

	ctx.decodeRequestHeader()

	// By protocol opcode & opaque same as client request
	ctx.response = ResponseHeader{
		magic:  ResponseMagic,
		opcode: ctx.request.opcode,
		status: NoErr,
		opaque: ctx.request.opaque,
	}

	switch ctx.request.opcode {
	case Set, SetQ, Add, AddQ:
		flags := unsafe.Slice(&ctx.flags[0], len(ctx.flags))
		exptime := unsafe.Slice(&ctx.exptime[0], len(ctx.exptime))

		if uint16(len(ctx.key)) < ctx.request.keyLen {
			ctx.key = make([]byte, ctx.request.keyLen)
		}
		key := unsafe.Slice(&ctx.key[0], ctx.request.keyLen)

		bodyLen := ctx.request.totalBody - uint32(ctx.request.keyLen) - uint32(ctx.request.extrasLen)
		__value := ctx.store.ValuePool.Get().(*[]byte)
		_value := *__value
		if len(_value) < int(bodyLen) {
			_value = append(_value, make([]byte, int(bodyLen)-len(_value))...)
		}
		value := unsafe.Slice(&_value[0], int(bodyLen))
		err_s := make([]error, 4)
		_, err_s[0] = ctx.rb.Read(flags)
		_, err_s[1] = ctx.rb.Read(exptime)
		_, err_s[2] = ctx.rb.Read(key)
		if bodyLen > 0 {
			_, err_s[3] = io.ReadFull(ctx.rb, value)
		}
		for _, err := range err_s {
			if err != nil {
				ctx.response.status = EInter
				ctx.Response()
				return err
			}
		}

		if ctx.debug {
			slog.Debug("Set/Add",
				"Flags", fmt.Sprintf("0x%08x", flags),
				"ExpTime", fmt.Sprintf("0x%08x", exptime),
				"Key", key,
			)
		}

		entry := memstore.MEntry{
			Key:     string(key[:]),
			ExpTime: binary.BigEndian.Uint32(exptime),
			Size:    bodyLen,
			Value:   value,
		}
		copy(unsafe.Slice(&entry.Flags[0], len(entry.Flags)), flags)

		if ctx.request.cas != 0 {
			_v, ok := ctx.store.Get(entry.Key)
			if ok {
				v := *_v
				if ctx.request.cas != v.Cas {
					ctx.response.status = Exist
					return ctx.Response()
				}
			}
		}

		if ctx.request.opcode == Add || ctx.request.opcode == AddQ {
			_, ok := ctx.store.Get(entry.Key)
			if ok {
				ctx.response.status = Exist
				// v := _v.(memstore.MEntry)
				ctx.Response()
				// ctx.wb.Write(v.value)
				return nil
			}
		}

		err = ctx.store.Set(entry.Key, &entry)
		if err != nil {
			return err
		}
		ctx.response.cas = entry.Cas

		if ctx.request.opcode == SetQ || ctx.request.opcode == AddQ {
			return nil
		}

		return ctx.Response()
	case Get, GetQ:
		if uint16(len(ctx.key)) < ctx.request.keyLen {
			ctx.key = make([]byte, ctx.request.keyLen)
		}
		key := unsafe.Slice(&ctx.key[0], ctx.request.keyLen)
		_, err = ctx.rb.Read(key)
		if err != nil {
			ctx.response.status = EInter
			ctx.Response()
			return err
		}

		_key := unsafe.String(&key[0], len(key))
		_v, ok := ctx.store.Get(_key)

		if !ok {
			if ctx.request.opcode == GetQ {
				return nil
			}
			ctx.response.status = NEnt
			return ctx.Response()
		}
		v := _v

		ctx.response.cas = v.Cas
		ctx.response.extrasLen = 4
		ctx.response.totalBody = 4 + uint32(len(v.Value))
		flags := unsafe.Slice(&v.Flags[0], len(v.Flags))

		ctx.Response(flags, unsafe.Slice(&v.Value[0], v.Size))

		return nil
	case Flush, FlushQ:
		exptime := unsafe.Slice(&ctx.exptime[0], len(ctx.exptime))
		if ctx.request.extrasLen == 4 {
			_, err = ctx.rb.Read(exptime)
			if err != nil {
				ctx.response.status = EInter
				ctx.Response()
				return err
			}

			slog.Debug("ExpTim: 0x%08x\n", exptime)
		}

		ctx.store.Flush()

		if ctx.request.opcode == FlushQ {
			return nil
		}

		return ctx.Response()
	case Quit:
		ctx.Response()
		return fmt.Errorf("Quit")
	case QuitQ:
		return fmt.Errorf("QuitQ")
	case NoOp:
		return ctx.Response()
	}

	return fmt.Errorf("not implemented opcode: 0x%02x", ctx.request.opcode)
}

func (ctx *Processor) ReadRequest() error {
	raw_request := unsafe.Slice(&ctx.raw_request[0], len(ctx.raw_request))
	_, err := ctx.rb.Read(raw_request)

	if err != nil {
		return err
	}

	return nil
}

func (ctx *Processor) Response(bytes ...[]byte) error {
	ctx.encodeResponseHeader()
	raw_response := unsafe.Slice(&ctx.raw_response[0], len(ctx.raw_request))

	_, err := ctx.wb.Write(raw_response)
	if err != nil {
		return err
	}

	for _, arg := range bytes {
		_, err = ctx.wb.Write(arg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (ctx *Processor) decodeRequestHeader() {
	ctx.request.magic = Magic(ctx.raw_request[0])
	ctx.request.opcode = OpcodeType(ctx.raw_request[1])
	ctx.request.keyLen = binary.BigEndian.Uint16(ctx.raw_request[2:4])
	ctx.request.extrasLen = ctx.raw_request[4]
	ctx.request.dataType = ctx.raw_request[5]
	ctx.request.vBucketId = binary.BigEndian.Uint16(ctx.raw_request[6:8])
	ctx.request.totalBody = binary.BigEndian.Uint32(ctx.raw_request[8:12])
	ctx.request.opaque[0] = ctx.raw_request[12]
	ctx.request.opaque[1] = ctx.raw_request[13]
	ctx.request.opaque[2] = ctx.raw_request[14]
	ctx.request.opaque[3] = ctx.raw_request[15]
	ctx.request.cas = binary.BigEndian.Uint64(ctx.raw_request[16:24])

	if ctx.debug {
		slog.Debug("Request",
			"Magic", fmt.Sprintf("0x%02x", ctx.request.magic),
			"Opcode", fmt.Sprintf("0x%02x", ctx.request.opcode),
			"KeyLen", fmt.Sprintf("0x%04x", ctx.request.keyLen),
			"Extra", fmt.Sprintf("0x%02x", ctx.request.extrasLen),
			"DType", fmt.Sprintf("0x%02x", ctx.request.dataType),
			"vBuckt", fmt.Sprintf("0x%04x", ctx.request.vBucketId),
			"TBody", fmt.Sprintf("0x%08x", ctx.request.totalBody),
			"Opaque", fmt.Sprintf("0x%08x", ctx.request.opaque),
			"CAS", fmt.Sprintf("0x%016x", ctx.request.cas),
		)
	}
}

func (ctx *Processor) encodeResponseHeader() {
	ctx.raw_response[0] = byte(ctx.response.magic)
	ctx.raw_response[1] = byte(ctx.response.opcode)

	// Write response directly to buffer
	keyLen := unsafe.Slice(&ctx.raw_response[2], 2)
	binary.BigEndian.PutUint16(keyLen, uint16(ctx.response.keyLen))

	ctx.raw_response[4] = ctx.response.extrasLen
	ctx.raw_response[5] = ctx.response.dataType

	status := unsafe.Slice(&ctx.raw_response[6], 2)
	binary.BigEndian.PutUint16(status, uint16(ctx.response.status))

	totalBody := unsafe.Slice(&ctx.raw_response[8], 4)
	binary.BigEndian.PutUint32(totalBody, uint32(ctx.response.totalBody))

	ctx.raw_response[12] = ctx.response.opaque[0]
	ctx.raw_response[13] = ctx.response.opaque[1]
	ctx.raw_response[14] = ctx.response.opaque[2]
	ctx.raw_response[15] = ctx.response.opaque[3]

	cas := unsafe.Slice(&ctx.raw_response[16], 8)
	binary.BigEndian.PutUint64(cas, uint64(ctx.response.cas))

	if ctx.debug {
		slog.Debug("Response",
			"Magic", fmt.Sprintf("0x%02x", ctx.raw_response[0]),
			"Opcode", fmt.Sprintf("0x%02x", ctx.raw_response[1]),
			"KeyLen", fmt.Sprintf("0x%04x", ctx.raw_response[2:4]),
			"Extra", fmt.Sprintf("0x%02x", ctx.raw_response[4]),
			"DType", fmt.Sprintf("0x%02x", ctx.raw_response[5]),
			"Status", fmt.Sprintf("0x%04x", ctx.raw_response[6:8]),
			"BodyL", fmt.Sprintf("0x%08x", ctx.raw_response[8:12]),
			"Opaque", fmt.Sprintf("0x%08x", ctx.raw_response[12:16]),
			"CAS", fmt.Sprintf("0x%016x", ctx.raw_response[16:24]),
		)
	}
}
