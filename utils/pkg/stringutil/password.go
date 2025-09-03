/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package stringutil

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
)

const (
	lowerLetters = "abcdefghijklmnopqrstuvwxyz"
	highLetters  = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	numbers      = "0123456789"
	marks        = "~!@#$%^&*()-_=+|[{}];:,<.>"
)

var pwdSource = [...]string{lowerLetters, highLetters, numbers, marks}

// Password generate password with length, a combination of following characters:
// lowercase letters, uppercase letters, numbers, marks
func Password(length int) string {
	pwd := strings.Builder{}
	for i := 0; i < length; i++ {
		idx := i % len(pwdSource)
		var l int
		switch idx {
		case 0, 1:
			l = len(lowerLetters)
		case 2:
			l = len(numbers)
		default:
			l = len(marks)
		}
		ch := pwdSource[idx][randUint32()%uint32(l)]
		pwd.WriteByte(ch)
	}
	return pwd.String()
}

func randUint32() uint32 {
	var k uint32
	if err := binary.Read(rand.Reader, binary.LittleEndian, &k); err != nil {
		return 0
	}
	return k
}
