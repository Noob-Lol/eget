package sdk

import (
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestFindBuiltinConfigResolvesNamesAndAliases(t *testing.T) {
	goOfficial, ok := FindBuiltinConfig("golang", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Eq(t, "go", goOfficial.Name)
	assert.Eq(t, BuiltinConfigOfficial, goOfficial.Source)
	assert.Eq(t, "https://go.dev/dl/", *goOfficial.Section.IndexURL)

	jdkMirror, ok := FindBuiltinConfig("java", BuiltinConfigMirror)
	assert.True(t, ok)
	assert.Eq(t, "jdk", jdkMirror.Name)
	assert.Eq(t, BuiltinConfigMirror, jdkMirror.Source)
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

	goMirror, ok := FindBuiltinConfig("go", BuiltinConfigMirror)
	assert.True(t, ok)
	assert.Eq(t, "https://mirrors.aliyun.com/golang/go{version}.{os}-{arch}.{ext}", *goMirror.Section.URLTemplate)

	jdkOfficial, ok := FindBuiltinConfig("jdk", BuiltinConfigOfficial)
	assert.True(t, ok)
	assert.Nil(t, jdkOfficial.Section.URLTemplate)
	assert.Eq(t, "https://jdk.java.net/archive/", *jdkOfficial.Section.IndexURL)
	assert.Eq(t, "openjdk-{version}_{os}-{arch}_bin.{ext}", *jdkOfficial.Section.FilenamePattern)

	jdkMirror, ok := FindBuiltinConfig("jdk", BuiltinConfigMirror)
	assert.True(t, ok)
	assert.Eq(t, "https://mirrors.huaweicloud.com/openjdk/{version}/openjdk-{version}_{os}-{arch}_bin.{ext}", *jdkMirror.Section.URLTemplate)
}
