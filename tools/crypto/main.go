/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"flag"
	"fmt"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/crypto"
)

type options struct {
	secret    string
	input     string
	doEncrypt bool
	doDecrypt bool
}

func (opt *options) InitFlags() error {
	flag.StringVar(&opt.secret, "key", "", "the secret key")
	flag.StringVar(&opt.input, "input", "", "the input to be processed")
	flag.BoolVar(&opt.doEncrypt, "e", false, "do encryption")
	flag.BoolVar(&opt.doDecrypt, "d", false, "do decryption")
	flag.Parse()

	if opt.secret == "" || opt.input == "" ||
		(opt.doEncrypt == false && opt.doDecrypt == false) {
		flag.PrintDefaults()
		return fmt.Errorf("failed to parse flag")
	}
	return nil
}

func main() {
	opt := options{}
	if opt.InitFlags() != nil {
		return
	}
	if opt.doEncrypt {
		data, err := crypto.Encrypt([]byte(opt.input), []byte(opt.secret))
		if err != nil {
			fmt.Println("failed to encrypt input", err)
			return
		}
		fmt.Println("Encrypted message:", data)
	} else if opt.doDecrypt {
		data, err := crypto.Decrypt(opt.input, []byte(opt.secret))
		if err != nil {
			fmt.Println("failed to decrypt input", err)
			return
		}
		fmt.Println("Decrypted message:", string(data))
	}
}
