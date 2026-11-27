package sharedcount

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// replyError is a Redis protocol error reply ("-ERR ..."). It is never retried on
// a fresh connection: the server answered, so the failure is not transport-level.
type replyError struct{ msg string }

func (e *replyError) Error() string { return "redis error: " + e.msg }

func isReplyError(err error) bool {
	var re *replyError
	return errors.As(err, &re)
}

func redisDo(conn net.Conn, parts ...string) error {
	_, err := redisDoRaw(conn, parts...)
	return err
}

func redisDoInt(conn net.Conn, parts ...string) (int64, error) {
	raw, err := redisDoRaw(conn, parts...)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

func redisDoIntNullable(conn net.Conn, parts ...string) (int64, error) {
	raw, err := redisDoRaw(conn, parts...)
	if err != nil {
		return 0, err
	}
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func redisDoRaw(conn net.Conn, parts ...string) (string, error) {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(parts)))
	b.WriteString("\r\n")
	for _, p := range parts {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(p)))
		b.WriteString("\r\n")
		b.WriteString(p)
		b.WriteString("\r\n")
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	return redisRead(conn)
}

func redisRead(conn net.Conn) (string, error) {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil && len(buf) == 0 {
			return "", err
		}
		if len(buf) == 0 {
			continue
		}
		switch buf[0] {
		case '+':
			if i := indexCRLF(buf); i >= 0 {
				return string(buf[1:i]), nil
			}
		case '-':
			if i := indexCRLF(buf); i >= 0 {
				return "", &replyError{msg: string(buf[1:i])}
			}
		case ':':
			if i := indexCRLF(buf); i >= 0 {
				return string(buf[1:i]), nil
			}
		case '$':
			// bulk: $<len>\r\n<data>\r\n or hmtBc1\r\n
			if i := indexCRLF(buf); i >= 0 {
				lenStr := string(buf[1:i])
				if lenStr == "-1" {
					return "", nil
				}
				ln, err := strconv.Atoi(lenStr)
				if err != nil {
					return "", err
				}
				start := i + 2
				if len(buf) >= start+ln+2 {
					return string(buf[start : start+ln]), nil
				}
			}
		default:
			return "", fmt.Errorf("unexpected redis reply prefix %q", buf[0])
		}
		if err != nil {
			return "", err
		}
	}
}

func indexCRLF(b []byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == 13 && b[i+1] == 10 {
			return i
		}
	}
	return -1
}
