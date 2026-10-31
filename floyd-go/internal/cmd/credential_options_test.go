package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDocumentedCredentialOptions(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"--proxy-user/separated", []string{"--proxy-user", "me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-user/equals", []string{"--proxy-user=me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-user/nested", []string{"--env=--proxy-user=me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-header/separated", []string{"--proxy-header", "Proxy-Authorization: Bearer OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-header/equals", []string{"--proxy-header=Proxy-Authorization: Bearer OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-header/nested", []string{"--env=--proxy-header=Proxy-Authorization: Bearer OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--oauth2-bearer/separated", []string{"--oauth2-bearer", "OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--oauth2-bearer/equals", []string{"--oauth2-bearer=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--oauth2-bearer/nested", []string{"--env=--oauth2-bearer=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--pass/separated", []string{"--pass", "OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--pass/equals", []string{"--pass=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--pass/nested", []string{"--env=--pass=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-pass/separated", []string{"--proxy-pass", "OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-pass/equals", []string{"--proxy-pass=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-pass/nested", []string{"--env=--proxy-pass=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--cert/separated", []string{"--cert", "client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--cert/equals", []string{"--cert=client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--cert/nested", []string{"--env=--cert=client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-cert/separated", []string{"--proxy-cert", "client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-cert/equals", []string{"--proxy-cert=client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-cert/nested", []string{"--env=--proxy-cert=client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--cookie/separated", []string{"--cookie", "sid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--cookie/equals", []string{"--cookie=sid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--cookie/nested", []string{"--env=--cookie=sid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy/separated", []string{"--proxy", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--proxy/equals", []string{"--proxy=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--proxy/nested", []string{"--env=--proxy=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--preproxy/separated", []string{"--preproxy", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--preproxy/equals", []string{"--preproxy=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--preproxy/nested", []string{"--env=--preproxy=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--proxy1.0/separated", []string{"--proxy1.0", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--proxy1.0/equals", []string{"--proxy1.0=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--proxy1.0/nested", []string{"--env=--proxy1.0=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--ftp-account/separated", []string{"--ftp-account", "OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--ftp-account/equals", []string{"--ftp-account=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--ftp-account/nested", []string{"--env=--ftp-account=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-key/separated", []string{"--proxy-key", "pkcs11:object=x;pin-value=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-key/equals", []string{"--proxy-key=pkcs11:object=x;pin-value=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--proxy-key/nested", []string{"--env=--proxy-key=pkcs11:object=x;pin-value=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-U/separated", []string{"-U", "me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-U/attached", []string{"-Ume:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-U/grouped", []string{"-svUme:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-U/grouped-separated", []string{"-svU", "me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-U/nested", []string{"--env=-svUme:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-b/separated", []string{"-b", "sid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-b/attached", []string{"-bsid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-b/grouped", []string{"-svbsid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-b/grouped-separated", []string{"-svb", "sid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-b/nested", []string{"--env=-svbsid=OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-E/separated", []string{"-E", "client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-E/attached", []string{"-Eclient.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-E/grouped", []string{"-svEclient.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-E/grouped-separated", []string{"-svE", "client.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-E/nested", []string{"--env=-svEclient.pem:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"-x/separated", []string{"-x", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"-x/attached", []string{"-xme:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"-x/grouped", []string{"-svxme:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"-x/grouped-separated", []string{"-svx", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"-x/nested", []string{"--env=-svxme:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--socks5/separated", []string{"--socks5", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--socks5/equals", []string{"--socks5=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--socks5/nested", []string{"--env=--socks5=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--socks5-hostname/separated", []string{"--socks5-hostname", "me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--socks5-hostname/equals", []string{"--socks5-hostname=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--socks5-hostname/nested", []string{"--env=--socks5-hostname=me:OPTION_TEST_CREDENTIAL@127.0.0.1:9", "--port", "1234"}},
		{"--expand-proxy-user/separated", []string{"--expand-proxy-user", "me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--expand-proxy-user/equals", []string{"--expand-proxy-user=me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
		{"--expand-proxy-user/nested", []string{"--env=--expand-proxy-user=me:OPTION_TEST_CREDENTIAL", "--port", "1234"}},
	}
	for _, field := range []string{"args", "arguments"} {
		for _, tc := range cases {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				original := append([]string(nil), tc.args...)
				b, err := redactedJSON(map[string]any{field: tc.args})
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(b), "OPTION_TEST_CREDENTIAL") {
					t.Fatal("credential remains visible")
				}
				var got map[string][]string
				if err := json.Unmarshal(b, &got); err != nil {
					t.Fatal(err)
				}
				a := got[field]
				if len(a) != len(tc.args) || a[len(a)-2] != "--port" || a[len(a)-1] != "1234" {
					t.Fatal("noncredential arguments changed")
				}
				if !reflect.DeepEqual(tc.args, original) {
					t.Fatal("original input mutated")
				}
			})
		}
	}
}

func TestAuthenticationTogglesDoNotConsumeArguments(t *testing.T) {
	for _, flag := range []string{"--basic", "--digest", "--proxy-basic", "--anyauth", "--tlsuser", "--mail-auth"} {
		args := []any{flag, "visible", "--port", "1234"}
		original := append([]any(nil), args...)
		redactCredentialArgs(args)
		if !reflect.DeepEqual(args, original) {
			t.Errorf("%s changed harmless arguments", flag)
		}
	}
}
