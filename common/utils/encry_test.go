package utils

import (
	"fmt"
	"testing"
	"time"
)

func TestSHA256Sum(t *testing.T) {
	data := []byte("hello world")
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	result := SHA256Sum(data)
	if result != expected {
		t.Errorf("SHA256Sum() = %v, want %v", result, expected)
	}
}

func TestSHA256SumString(t *testing.T) {
	plainText := "hello world"
	expected := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	result := SHA256SumString(plainText)
	if result != expected {
		t.Errorf("SHA256SumString() = %v, want %v", result, expected)
	}
}

func TestMD5Sum(t *testing.T) {
	plainText := "hello world"
	expected := "5eb63bbbe01eeed093cb22bb8f5acdc3"
	result := MD5Sum(plainText)
	if result != expected {
		t.Errorf("MD5Sum() = %v, want %v", result, expected)
	}
}

func TestSHA512Sum(t *testing.T) {
	plainText := "hello world"
	expected := "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f"
	result := SHA512Sum(plainText)
	if result != expected {
		t.Errorf("SHA512Sum() = %v, want %v", result, expected)
	}
}

func TestAESHelper(t *testing.T) {
	// 生成测试密钥和IV
	key, err := GenerateAESKey(32)
	if err != nil {
		t.Fatalf("GenerateAESKey failed: %v", err)
	}

	iv, err := GenerateIV()
	if err != nil {
		t.Fatalf("GenerateIV failed: %v", err)
	}

	// 创建AES助手
	aes, err := NewAESHelper(key, iv)
	if err != nil {
		t.Fatalf("NewAESHelper failed: %v", err)
	}

	plainText := "Hello, World! This is a test message."

	// 测试Base64加密解密
	encrypted, err := aes.EncryptBase64(plainText)
	if err != nil {
		t.Fatalf("EncryptBase64 failed: %v", err)
	}

	decrypted, err := aes.DecryptBase64(encrypted)
	if err != nil {
		t.Fatalf("DecryptBase64 failed: %v", err)
	}

	if decrypted != plainText {
		t.Errorf("DecryptBase64() = %v, want %v", decrypted, plainText)
	}

	// 测试Hex加密解密
	encryptedHex, err := aes.EncryptHex(plainText)
	if err != nil {
		t.Fatalf("EncryptHex failed: %v", err)
	}

	decryptedHex, err := aes.DecryptHex(encryptedHex)
	if err != nil {
		t.Fatalf("DecryptHex failed: %v", err)
	}

	if decryptedHex != plainText {
		t.Errorf("DecryptHex() = %v, want %v", decryptedHex, plainText)
	}
}

func TestAESHelperGCM(t *testing.T) {
	// 生成测试密钥
	key, err := GenerateAESKey(32)
	if err != nil {
		t.Fatalf("GenerateAESKey failed: %v", err)
	}

	plainText := "Hello, World! This is a test message for GCM mode."

	// 测试GCM加密解密（使用新的EncryptGCM/DecryptGCM函数）
	encrypted, nonce, err := EncryptGCM(key, plainText)
	if err != nil {
		t.Fatalf("EncryptGCM failed: %v", err)
	}

	decrypted, err := DecryptGCM(key, nonce, encrypted)
	if err != nil {
		t.Fatalf("DecryptGCM failed: %v", err)
	}

	fmt.Println(decrypted)
	if decrypted != plainText {
		t.Errorf("DecryptGCM() = %v, want %v", decrypted, plainText)
	}
}

func TestRSAHelper(t *testing.T) {
	// 生成RSA密钥对
	privateKey, _, err := GenerateRSAKeyPair(2048)
	if err != nil {
		t.Fatalf("GenerateRSAKeyPair failed: %v", err)
	}

	// 转换为PEM格式
	privateKeyPEM, publicKeyPEM, err := RSAKeyPairToPEM(privateKey)
	if err != nil {
		t.Fatalf("RSAKeyPairToPEM failed: %v", err)
	}

	// 创建RSA助手（使用私钥）
	rsaHelper, err := NewRSAHelper(privateKeyPEM, "")
	if err != nil {
		t.Fatalf("NewRSAHelper with private key failed: %v", err)
	}

	plainText := []byte("Hello, World! This is a test message for RSA encryption.")

	// 测试加密解密
	encrypted, err := rsaHelper.Encrypt(plainText)
	if err != nil {
		t.Fatalf("RSA Encrypt failed: %v", err)
	}

	decrypted, err := rsaHelper.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("RSA Decrypt failed: %v", err)
	}

	if string(decrypted) != string(plainText) {
		t.Errorf("RSA Decrypt() = %v, want %v", string(decrypted), string(plainText))
	}

	// 测试仅公钥的RSA助手
	rsaHelperPub, err := NewRSAHelper("", publicKeyPEM)
	if err != nil {
		t.Fatalf("NewRSAHelper with public key failed: %v", err)
	}

	// 使用公钥加密
	encryptedPub, err := rsaHelperPub.Encrypt(plainText)
	if err != nil {
		t.Fatalf("RSA Encrypt with public key failed: %v", err)
	}

	// 使用私钥解密
	decryptedPub, err := rsaHelper.Decrypt(encryptedPub)
	if err != nil {
		t.Fatalf("RSA Decrypt with private key failed: %v", err)
	}

	if string(decryptedPub) != string(plainText) {
		t.Errorf("RSA Decrypt with private key() = %v, want %v", string(decryptedPub), string(plainText))
	}
}

func TestHMACHelper(t *testing.T) {
	key := []byte("test-secret-key")
	hmac := NewHMACHelper(key)

	data := []byte("Hello, World! This is a test message for HMAC.")

	// 测试HMAC256
	mac := hmac.HMAC256(data)
	if len(mac) == 0 {
		t.Error("HMAC256 returned empty result")
	}

	// 测试HMAC256Hex
	macHex := hmac.HMAC256Hex(data)
	if macHex == "" {
		t.Error("HMAC256Hex returned empty result")
	}

	// 测试HMAC256Base64
	macBase64 := hmac.HMAC256Base64(data)
	if macBase64 == "" {
		t.Error("HMAC256Base64 returned empty result")
	}

	// 测试验证
	if !hmac.VerifyHMAC256(data, mac) {
		t.Error("VerifyHMAC256 failed for valid MAC")
	}

	// 测试无效MAC
	invalidMAC := []byte("invalid-mac")
	if hmac.VerifyHMAC256(data, invalidMAC) {
		t.Error("VerifyHMAC256 should fail for invalid MAC")
	}
}

func TestPasswordHash(t *testing.T) {
	password := "test-password-123"

	// 测试bcrypt
	hashedPassword, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hashedPassword == "" {
		t.Error("HashPassword returned empty result")
	}

	if !VerifyPassword(hashedPassword, password) {
		t.Error("VerifyPassword failed for correct password")
	}

	if VerifyPassword(hashedPassword, "wrong-password") {
		t.Error("VerifyPassword should fail for wrong password")
	}

	// 测试scrypt
	scryptHash, err := ScryptHash(password, nil)
	if err != nil {
		t.Fatalf("ScryptHash failed: %v", err)
	}

	if scryptHash == "" {
		t.Error("ScryptHash returned empty result")
	}

	if !ScryptVerify(scryptHash, password) {
		t.Error("ScryptVerify failed for correct password")
	}

	if ScryptVerify(scryptHash, "wrong-password") {
		t.Error("ScryptVerify should fail for wrong password")
	}
}

func TestJWT(t *testing.T) {
	secretKey := "test-secret-key"
	claims := JWTClaims{
		UserID:   "12345",
		Username: "testuser",
		Roles:    []string{"user", "admin"},
		Extra:    map[string]string{"department": "engineering"},
	}

	// 生成JWT
	token, err := GenerateJWT(claims, secretKey, time.Hour)
	if err != nil {
		t.Fatalf("GenerateJWT failed: %v", err)
	}

	if token == "" {
		t.Error("GenerateJWT returned empty token")
	}

	// 验证JWT
	verifiedClaims, err := VerifyJWT(token, secretKey)
	if err != nil {
		t.Fatalf("VerifyJWT failed: %v", err)
	}

	if verifiedClaims.UserID != claims.UserID {
		t.Errorf("VerifyJWT UserID = %v, want %v", verifiedClaims.UserID, claims.UserID)
	}

	if verifiedClaims.Username != claims.Username {
		t.Errorf("VerifyJWT Username = %v, want %v", verifiedClaims.Username, claims.Username)
	}

	// 测试无效token
	_, err = VerifyJWT("invalid-token", secretKey)
	if err == nil {
		t.Error("VerifyJWT should fail for invalid token")
	}

	// 测试错误的密钥
	_, err = VerifyJWT(token, "wrong-secret-key")
	if err == nil {
		t.Error("VerifyJWT should fail for wrong secret key")
	}
}

func TestUtilityFunctions(t *testing.T) {
	// 测试随机字符串生成
	randomStr, err := GenerateRandomString(16)
	if err != nil {
		t.Fatalf("GenerateRandomString failed: %v", err)
	}

	if len(randomStr) != 16 {
		t.Errorf("GenerateRandomString length = %v, want 16", len(randomStr))
	}

	// 测试随机字节生成
	randomBytes, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes failed: %v", err)
	}

	if len(randomBytes) != 32 {
		t.Errorf("GenerateRandomBytes length = %v, want 32", len(randomBytes))
	}

	// 测试AES密钥生成
	aesKey, err := GenerateAESKey(32)
	if err != nil {
		t.Fatalf("GenerateAESKey failed: %v", err)
	}

	if len(aesKey) != 32 {
		t.Errorf("GenerateAESKey length = %v, want 32", len(aesKey))
	}

	// 测试IV生成
	iv, err := GenerateIV()
	if err != nil {
		t.Fatalf("GenerateIV failed: %v", err)
	}

	if len(iv) != 16 {
		t.Errorf("GenerateIV length = %v, want 16", len(iv))
	}

	// 测试Base64编码解码
	testData := []byte("Hello, World!")
	encoded := Base64Encode(testData)
	decoded, err := Base64Decode(encoded)
	if err != nil {
		t.Fatalf("Base64Decode failed: %v", err)
	}

	if string(decoded) != string(testData) {
		t.Errorf("Base64Decode() = %v, want %v", string(decoded), string(testData))
	}

	// 测试Hex编码解码
	hexEncoded := HexEncode(testData)
	hexDecoded, err := HexDecode(hexEncoded)
	if err != nil {
		t.Fatalf("HexDecode failed: %v", err)
	}

	if string(hexDecoded) != string(testData) {
		t.Errorf("HexDecode() = %v, want %v", string(hexDecoded), string(testData))
	}

	// 测试常量时间比较
	if !ConstantTimeCompare("hello", "hello") {
		t.Error("ConstantTimeCompare should return true for equal strings")
	}

	if ConstantTimeCompare("hello", "world") {
		t.Error("ConstantTimeCompare should return false for different strings")
	}
}

func TestAESHelperErrorHandling(t *testing.T) {
	// 测试无效密钥长度
	invalidKey := []byte("short")
	iv := make([]byte, 16)

	_, err := NewAESHelper(invalidKey, iv)
	if err == nil {
		t.Error("NewAESHelper should fail for invalid key length")
	}

	// 测试无效IV长度
	key := make([]byte, 32)
	invalidIV := []byte("short")

	_, err = NewAESHelper(key, invalidIV)
	if err == nil {
		t.Error("NewAESHelper should fail for invalid IV length")
	}
}

func TestRSAHelperErrorHandling(t *testing.T) {
	// 测试无效PEM格式
	_, err := NewRSAHelper("invalid-pem", "")
	if err == nil {
		t.Error("NewRSAHelper should fail for invalid PEM format")
	}

	// 测试空密钥
	_, err = NewRSAHelper("", "")
	if err == nil {
		t.Error("NewRSAHelper should fail when no keys provided")
	}
}

func TestGenerateAESKeyErrorHandling(t *testing.T) {
	// 测试无效密钥大小
	_, err := GenerateAESKey(64)
	if err == nil {
		t.Error("GenerateAESKey should fail for invalid key size")
	}
}
