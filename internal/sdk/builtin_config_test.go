package sdk

import (
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestFindBuiltinConfigResolvesNamesAndAliases(t *testing.T) {
	goOfficial, ok := FindBuiltinConfig("golang", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Eq(t, "go", goOfficial.Name)
	assert.Eq(t, BuiltinConfigOfficial, goOfficial.Source)
	assert.Eq(t, "https://go.dev/dl/", *goOfficial.Section.IndexURL)

	jdkMirror, ok := FindBuiltinConfig("java", BuiltinConfigHuawei)
	assert.True(t, ok)
	assert.Eq(t, "jdk", jdkMirror.Name)
	assert.Eq(t, BuiltinConfigHuawei, jdkMirror.Source)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/", *jdkMirror.Section.IndexURL)

	_, ok = FindBuiltinConfig("ruby", BuiltinConfigOfficial)
	assert.False(t, ok)
}

func TestBuiltinConfigNames(t *testing.T) {
	assert.Eq(t, []string{"go", "node", "jdk"}, BuiltinConfigNames())
}

func TestBuiltinOfficialAndMirrorTemplatesUseExpectedURLs(t *testing.T) {
	goOfficial, ok := FindBuiltinConfig("go", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Eq(t, "https://go.dev/dl/go{version}.{os}-{arch}.{ext}", *goOfficial.Section.URLTemplate)

	goMirror, ok := FindBuiltinConfig("go", BuiltinConfigAliyun)
	assert.True(t, ok)
	assert.Eq(t, "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}", *goMirror.Section.URLTemplate)

	jdkOfficial, ok := FindBuiltinConfig("jdk", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Nil(t, jdkOfficial.Section.URLTemplate)
	assert.Eq(t, "https://jdk.java.net/archive/", *jdkOfficial.Section.IndexURL)
	assert.Eq(t, "openjdk-{version}_{os}-{arch}_bin.{ext}", *jdkOfficial.Section.FilenamePattern)

	jdkMirror, ok := FindBuiltinConfig("jdk", BuiltinConfigHuawei)
	assert.True(t, ok)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}", *jdkMirror.Section.URLTemplate)
}

func TestBuiltinNamedMirrorTemplatesUseExpectedURLs(t *testing.T) {
	_, ok := FindBuiltinConfig("jdk", BuiltinConfigAliyun)
	assert.False(t, ok)

	zuluJDK, ok := FindBuiltinConfig("java", BuiltinConfigZulu)
	assert.True(t, ok)
	assert.Eq(t, "jdk", zuluJDK.Name)
	assert.Eq(t, BuiltinConfigZulu, zuluJDK.Source)
	assert.Eq(t, "jdk/zulu-{version}", *zuluJDK.Section.Target)
	assert.Eq(t, "https://api.azul.com/metadata/v1/zulu/packages?java_package_type=jdk&release_status=ga&availability_type=CA", *zuluJDK.Section.IndexURL)
	assert.Eq(t, "json", *zuluJDK.Section.IndexFormat)
	assert.Eq(t, "zulu-json", *zuluJDK.Section.IndexParser)
	assert.Eq(t, "win", zuluJDK.Section.OSMap["windows"])
	assert.Eq(t, "macosx", zuluJDK.Section.OSMap["darwin"])
}
