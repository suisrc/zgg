//
// 证书管理和生成

package crt

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

type CertConfig struct {
	CommonName string                 `json:"CN"`
	SignKey    SignKey                `json:"key"`
	CaProfile  SignProfile            `json:"CA"`
	Profiles   map[string]SignProfile `json:"profiles"`
}

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
	Expiry      string       `json:"expiry"`
	CommonName  string       `json:"CN"`
	SignKey     SignKey      `json:"key"`
	SubjectName *SignSubject `json:"name"`
}

//===========================================================================

type SignResult struct {
	Crt string `json:"crt"`
	Key string `json:"key"`
}

//===========================================================================

// 合并配置
func (aa *CertConfig) Merge(bb *CertConfig) bool {
	update := false
	if bb.SignKey.Size > 0 && bb.SignKey.Size != aa.SignKey.Size {
		aa.SignKey.Size = bb.SignKey.Size
		update = true
	}
	for bKey, bVal := range bb.Profiles {
		aa.Profiles[bKey] = bVal
		update = true
	}
	return update
}

// String...
func (aa *CertConfig) String() string {
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
func IsPemExpired(pemStr string) (bool, error) {
	pemBlk, _ := pem.Decode([]byte(pemStr))
	if pemBlk == nil {
		return true, fmt.Errorf("invalid ca.crt, pem")
	}
	pemCrt, err := x509.ParseCertificate(pemBlk.Bytes)
	if err != nil {
		return true, fmt.Errorf("invalid ca.crt, bytes")
	}

	if time.Now().After(pemCrt.NotAfter) {
		return true, nil
	}
	return false, nil
}

// 构建的证书是虚假的，默认有效期99年
func CreateCA(config *CertConfig /*, notAfter time.Time*/) (SignResult, error) {
	profile := &config.CaProfile
	// ----------------------------------------------------------------------------

	subject := pkix.Name{ //Name代表一个X.509识别名。只包含识别名的公共属性，额外的属性被忽略。
		CommonName:         config.CommonName,
		Country:            StrToArray(profile.SubjectName.Country),
		Province:           StrToArray(profile.SubjectName.Province),
		Locality:           StrToArray(profile.SubjectName.Locality),
		Organization:       StrToArray(profile.SubjectName.Organization),
		OrganizationalUnit: StrToArray(profile.SubjectName.OrganizationUnit),
	}
	// 过期时间
	notAfter, err := GetExpiredTime(profile.Expiry, (20*365 + 24))
	if err != nil {
		return SignResult{}, err
	}
	// 加密方式
	algorithm := x509.SHA256WithRSA
	if config.SignKey.Size >= 4096 {
		algorithm = x509.SHA512WithRSA
	} else if config.SignKey.Size >= 2048 {
		algorithm = x509.SHA384WithRSA
	}
	pkey, _ := rsa.GenerateKey(rand.Reader, config.SignKey.Size) //生成一对具有指定字位数的RSA密钥

	sermax := new(big.Int).Lsh(big.NewInt(1), 128) //把 1 左移 128 位，返回给 big.Int
	serial, _ := rand.Int(rand.Reader, sermax)     //返回在 [0, max) 区间均匀随机分布的一个随机值
	pder := x509.Certificate{
		SerialNumber: serial, // SerialNumber 是 CA 颁布的唯一序列号，在此使用一个大随机数来代表它
		IsCA:         true,
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		BasicConstraintsValid: true,
		SignatureAlgorithm:    algorithm, // SignatureAlgorithm 签名算法
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

// CreateCertificate 创建一个证书，默认有效期10年
func CreateCE(certConfig *CertConfig, commonName, profileKey string, keySize int, dns []string, ips []net.IP, caCrtPemBts, caKeyPemBts []byte) (SignResult, error) {
	if commonName == "" {
		if len(dns) == 1 {
			commonName = dns[0]
		} else if len(ips) == 1 {
			commonName = ips[0].String()
		} else {
			return SignResult{}, fmt.Errorf("invalid commonName")
		}
	}
	// ----------------------------------------------------------------------------
	var profile *SignProfile
	// 获取证书配置
	if certConfig != nil {
		if pfile, ok := certConfig.Profiles[profileKey]; ok {
			profile = &pfile
		} else if pfile, ok = certConfig.Profiles["default"]; ok {
			profile = &pfile
		} else {
			return SignResult{}, fmt.Errorf("no profile: %s", profileKey)
		}
	} else {
		profile = &SignProfile{Expiry: "10y", SubjectName: &SignSubject{
			Organization:     "default",
			OrganizationUnit: "default",
		}}
	}
	if keySize == 0 {
		if certConfig != nil {
			keySize = certConfig.SignKey.Size
		} else {
			keySize = 2048
		}
	}
	// ----------------------------------------------------------------------------
	var caCrt *x509.Certificate
	var caKey *rsa.PrivateKey
	if caCrtPemBts != nil && caKeyPemBts != nil {
		var err error
		caCrtBlk, _ := pem.Decode(caCrtPemBts)
		if caCrtBlk == nil {
			return SignResult{}, fmt.Errorf("invalid ca.crt, pem")
		}
		caCrt, err = x509.ParseCertificate(caCrtBlk.Bytes)
		if err != nil {
			return SignResult{}, fmt.Errorf("invalid ca.crt, bts")
		}
		caKeyBlk, _ := pem.Decode(caKeyPemBts)
		if caKeyBlk == nil {
			return SignResult{}, fmt.Errorf("invalid ca.key, pem")
		}
		caKey, err = x509.ParsePKCS1PrivateKey(caKeyBlk.Bytes)
		if err != nil {
			return SignResult{}, fmt.Errorf("invalid ca.key, bts")
		}
	}
	// ----------------------------------------------------------------------------

	subject := pkix.Name{ //Name代表一个X.509识别名。只包含识别名的公共属性，额外的属性被忽略。
		CommonName:         commonName,
		Country:            StrToArray(profile.SubjectName.Country),
		Province:           StrToArray(profile.SubjectName.Province),
		Locality:           StrToArray(profile.SubjectName.Locality),
		Organization:       StrToArray(profile.SubjectName.Organization),
		OrganizationalUnit: StrToArray(profile.SubjectName.OrganizationUnit),
	}
	// 过期时间
	notAfter, err := GetExpiredTime(profile.Expiry, (10*365 + 2))
	if err != nil {
		return SignResult{}, err
	}
	// 加密方式
	algorithm := x509.SHA256WithRSA
	if certConfig != nil {
		if certConfig.SignKey.Size >= 4096 {
			algorithm = x509.SHA512WithRSA
		} else if certConfig.SignKey.Size >= 2048 {
			algorithm = x509.SHA384WithRSA
		}
	}
	//生成一对具有指定字位数的RSA密钥
	pkey, _ := rsa.GenerateKey(rand.Reader, keySize)
	if caKey != nil {
		caKey = pkey
	}

	sermax := new(big.Int).Lsh(big.NewInt(1), 128) //把 1 左移 128 位，返回给 big.Int
	serial, _ := rand.Int(rand.Reader, sermax)     //返回在 [0, max) 区间均匀随机分布的一个随机值
	pder := x509.Certificate{
		SerialNumber: serial,
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
			x509.ExtKeyUsageClientAuth,
		},
		SignatureAlgorithm: algorithm,
		DNSNames:           dns,
		IPAddresses:        ips,
	}
	if caKey == nil {
		caKey = pkey
		caCrt = &pder
	}
	// ----------------------------------------------------------------------------

	derBytes, err := x509.CreateCertificate(rand.Reader, &pder, caCrt, &pkey.PublicKey, caKey)
	if err != nil {
		return SignResult{}, nil
	}
	crtBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(pkey)})

	return SignResult{Crt: string(crtBytes), Key: string(keyBytes)}, nil
}
