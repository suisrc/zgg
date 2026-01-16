// Copyright 2026 suisrc. All rights reserved.
// Based on the path package, Copyright 2009 The Go Authors.
// Use of this source code is governed by a BSD-style license that can be found
// at https://github.com/suisrc/zgg/blob/main/LICENSE.

// 证书管理和生成

package tlsx

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"
)

type CertConfig map[string]SignProfile

type SignKey struct {
	Size int `json:"size"`
}

type SignSubject struct {
	Country          string `json:"C"`
	Province         string `json:"ST"`
	Locality         string `json:"L"`
	Organization     string `json:"O"`
	OrganizationUnit string `json:"OU"`
}

type SignProfile struct {
	Expiry      string      `json:"expiry"`
	KeySize     int         `json:"size"`
	SubjectName SignSubject `json:"name"`
}

//===========================================================================

type SignResult struct {
	Crt string `json:"crt"`
	Key string `json:"key"`
}

//===========================================================================

// 合并配置
func (aa CertConfig) Merge(bb CertConfig) bool {
	update := false
	for bKey, bVal := range bb {
		aa[bKey] = bVal
		update = true
	}
	return update
}

// String...
func (aa CertConfig) String() string {
	str, _ := json.Marshal(aa)
	return string(str)
}

// StrToArray...
func StrToArray(str string) []string {
	if str == "" {
		return nil
	}
	return []string{str}
}

// HashMd5 MD5哈希值
func HashMd5(b []byte) (string, error) {
	h := md5.New()
	_, err := h.Write(b)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func GetExpiredTime(str string, day int) (time.Time, error) {
	after := time.Now()
	if str == "" {
		after = after.Add(time.Duration(day) * 24 * time.Hour)
	} else if strings.HasSuffix(str, "h") {
		exp, _ := strconv.Atoi(str[:len(str)-1])
		after = after.Add(time.Duration(exp) * time.Hour)
	} else if strings.HasSuffix(str, "d") {
		exp, _ := strconv.Atoi(str[:len(str)-1])
		after = after.Add(time.Duration(exp) * 24 * time.Hour)
	} else if strings.HasSuffix(str, "y") {
		exp, _ := strconv.Atoi(str[:len(str)-1])
		after = after.Add(time.Duration(exp) * 365 * 24 * time.Hour)
	} else {
		return after, fmt.Errorf("invalid str: %s", str)
	}
	return after, nil
}

// IsPemExpired 判定正式否使过期
func IsPemExpired(pemStr string) (bool, time.Time, error) {
	now_ := time.Now()
	pemBlk, _ := pem.Decode([]byte(pemStr))
	if pemBlk == nil {
		return true, now_, fmt.Errorf("invalid ca.crt, pem")
	}
	pemCrt, err := x509.ParseCertificate(pemBlk.Bytes)
	if err != nil {
		return true, now_, fmt.Errorf("invalid ca.crt, bytes")
	}

	if now_.After(pemCrt.NotAfter) {
		return true, pemCrt.NotAfter, nil
	}
	return false, pemCrt.NotAfter, nil
}

// CreateCertificate
type cdata struct {
	Subject   pkix.Name
	NotAfter  time.Time
	KeySize   int
	Algorithm x509.SignatureAlgorithm
	CaKey     any
	CaCrt     *x509.Certificate
}

func _cdata(certConfig CertConfig, commonName string, caCrtPemBts, caKeyPemBts []byte) (*cdata, error) {
	var profile *SignProfile
	// 获取证书配置
	if certConfig != nil {
		if pfile, ok := certConfig[commonName]; ok {
			profile = &pfile
		} else if pfile, ok = certConfig["default"]; ok {
			profile = &pfile
		} else {
			return nil, fmt.Errorf("no profile: %s", commonName)
		}
	} else {
		profile = &SignProfile{Expiry: "10y", SubjectName: SignSubject{
			Organization:     "default",
			OrganizationUnit: "default",
		}}
	}
	keySize := 2048
	if certConfig != nil && profile.KeySize > 0 {
		keySize = profile.KeySize
	}
	// 过期时间
	notAfter, err := GetExpiredTime(profile.Expiry, (10*365 + 2))
	if err != nil {
		return nil, err
	}
	// ----------------------------------------------------------------------------
	var algorithm x509.SignatureAlgorithm
	var caCrt *x509.Certificate
	var caKey any

	if caCrtPemBts != nil && caKeyPemBts != nil {
		var err error
		caCrtBlk, _ := pem.Decode(caCrtPemBts)
		if caCrtBlk == nil {
			return nil, fmt.Errorf("invalid ca.crt, pem")
		}
		caCrt, err = x509.ParseCertificate(caCrtBlk.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid ca.crt, bts, %s", err.Error())
		}
		if notAfter.After(caCrt.NotAfter) {
			notAfter = caCrt.NotAfter // 使用CA证书的过期时间
		}

		caKeyBlk, _ := pem.Decode(caKeyPemBts)
		if caKeyBlk == nil {
			return nil, fmt.Errorf("invalid ca.key, pem")
		}
		if caKeyBlk.Type == "EC PRIVATE KEY" {
			caKey, err = x509.ParseECPrivateKey(caKeyBlk.Bytes)
			algorithm = x509.ECDSAWithSHA256
			if certConfig != nil {
				if keySize >= 4096 {
					algorithm = x509.ECDSAWithSHA512
				} else if keySize >= 2048 {
					algorithm = x509.ECDSAWithSHA384
				}
			}
		} else {
			caKey, err = x509.ParsePKCS1PrivateKey(caKeyBlk.Bytes)
			algorithm = x509.SHA256WithRSA
			if certConfig != nil {
				if keySize >= 4096 {
					algorithm = x509.SHA512WithRSA
				} else if keySize >= 2048 {
					algorithm = x509.SHA384WithRSA
				}
			}
		}
		// caKey, err = x509.ParsePKCS1PrivateKey(caKeyBlk.Bytes)
		if err != nil {
			return nil, fmt.Errorf("invalid ca.key, bts, %s", err.Error())
		}
	}
	if algorithm == x509.UnknownSignatureAlgorithm {
		algorithm = x509.SHA256WithRSA
	}
	// ----------------------------------------------------------------------------
	return &cdata{
		Subject: pkix.Name{ //Name代表一个X.509识别名。只包含识别名的公共属性，额外的属性被忽略。
			CommonName:         commonName,
			Country:            StrToArray(profile.SubjectName.Country),
			Province:           StrToArray(profile.SubjectName.Province),
			Locality:           StrToArray(profile.SubjectName.Locality),
			Organization:       StrToArray(profile.SubjectName.Organization),
			OrganizationalUnit: StrToArray(profile.SubjectName.OrganizationUnit),
		},
		NotAfter:  notAfter,
		KeySize:   keySize,
		Algorithm: algorithm,
		CaKey:     caKey,
		CaCrt:     caCrt,
	}, nil
}

// 构建的根CA证书，默认有效期99年
func CreateCA(certConfig CertConfig, commonName string) (SignResult, error) {
	ata, err := _cdata(certConfig, commonName, nil, nil)
	if err != nil {
		return SignResult{}, err
	}
	pkey, _ := rsa.GenerateKey(rand.Reader, ata.KeySize) //生成一对具有指定字位数的RSA密钥

	sermax := new(big.Int).Lsh(big.NewInt(1), 128) //把 1 左移 128 位，返回给 big.Int
	serial, _ := rand.Int(rand.Reader, sermax)     //返回在 [0, max) 区间均匀随机分布的一个随机值
	pder := x509.Certificate{
		SerialNumber: serial, // SerialNumber 是 CA 颁布的唯一序列号，在此使用一个大随机数来代表它
		IsCA:         true,
		Subject:      ata.Subject,
		NotBefore:    time.Now(),
		NotAfter:     ata.NotAfter,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		SignatureAlgorithm:    ata.Algorithm, // SignatureAlgorithm 签名算法
		MaxPathLen:            1,             // 允许中间 CA 证书路径长度为1
	}
	// ----------------------------------------------------------------------------
	//CreateCertificate基于模板创建一个新的证书, 第二个第三个参数相同，则证书是自签名的
	//返回的切片是DER编码的证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &pder, &pder, &pkey.PublicKey, pkey)
	if err != nil {
		return SignResult{}, nil
	}
	crtBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pkey)})

	return SignResult{Crt: string(crtBytes), Key: string(keyBytes)}, nil
}

// 构建中间CA证书，默认有效期10年
func CreateSA(certConfig CertConfig, commonName string, caCrtPemBts, caKeyPemBts []byte) (SignResult, error) {
	ata, err := _cdata(certConfig, commonName, caCrtPemBts, caKeyPemBts)
	if err != nil {
		return SignResult{}, err
	}
	subkey, _ := rsa.GenerateKey(rand.Reader, ata.KeySize) //生成一对具有指定字位数的RSA密钥

	sermax := new(big.Int).Lsh(big.NewInt(1), 128) //把 1 左移 128 位，返回给 big.Int
	serial, _ := rand.Int(rand.Reader, sermax)     //返回在 [0, max) 区间均匀随机分布的一个随机值
	subder := x509.Certificate{
		SerialNumber: serial,
		IsCA:         true,
		Subject:      ata.Subject,
		NotBefore:    time.Now(),
		NotAfter:     ata.NotAfter,
		KeyUsage:     x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		SignatureAlgorithm:    ata.Algorithm,
		MaxPathLen:            0,
	}
	// ----------------------------------------------------------------------------
	//CreateCertificate基于模板创建一个新的证书, 第二个第三个参数相同，则证书是自签名的
	//返回的切片是DER编码的证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &subder, ata.CaCrt, &subkey.PublicKey, ata.CaKey)
	if err != nil {
		return SignResult{}, nil
	}
	crtBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(subkey)})

	return SignResult{Crt: string(crtBytes), Key: string(keyBytes)}, nil

}

// 创建一个证书，默认有效期10年
func CreateCE(certConfig CertConfig, commonName string, dns []string, ips []net.IP, caCrtPemBts, caKeyPemBts []byte) (SignResult, error) {
	if commonName == "" {
		if len(dns) == 1 {
			commonName = dns[0]
		} else if len(ips) == 1 {
			commonName = ips[0].String()
		} else {
			return SignResult{}, fmt.Errorf("invalid commonName")
		}
	}
	ata, err := _cdata(certConfig, commonName, caCrtPemBts, caKeyPemBts)
	if err != nil {
		return SignResult{}, err
	}
	pkey, _ := rsa.GenerateKey(rand.Reader, ata.KeySize) //生成一对具有指定字位数的RSA密钥

	sermax := new(big.Int).Lsh(big.NewInt(1), 128) //把 1 左移 128 位，返回给 big.Int
	serial, _ := rand.Int(rand.Reader, sermax)     //返回在 [0, max) 区间均匀随机分布的一个随机值
	pder := x509.Certificate{
		SerialNumber: serial,
		Subject:      ata.Subject,
		NotBefore:    time.Now(),
		NotAfter:     ata.NotAfter,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		SignatureAlgorithm: ata.Algorithm,
		DNSNames:           dns,
		IPAddresses:        ips,
	}
	if ata.CaKey == nil {
		ata.CaKey = pkey
		ata.CaCrt = &pder
	}
	// ----------------------------------------------------------------------------
	derBytes, err := x509.CreateCertificate(rand.Reader, &pder, ata.CaCrt, &pkey.PublicKey, ata.CaKey)
	if err != nil {
		return SignResult{}, err
	}
	crtBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pkey)})

	return SignResult{Crt: string(crtBytes), Key: string(keyBytes)}, nil
}
