package preview

import (
	"fmt"
	"testing"
	"time"
)

func TestCreateEnvironment(t *testing.T) {
	err := CreatePreviewCache("counter2", "Counter")
	if err != nil {
		t.Fatal(err)
	}
}

func TestCopyDir(t *testing.T) {
	err := CopyDir("templates", "fuck")
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuildProjectForPreview(t *testing.T) {
	err := BuildProject(".xim/preview_cache")
	if err != nil {
		t.Fatal(err)
	}
}

func TestPreview(t *testing.T) {
	err := Component("localhost:8080", "counter", "Counter")
	if err != nil {
		t.Fatal(err)
	}
}

func TestHttpServe(t *testing.T) {
	assets := &Assets{
		static:   &embedAssets,
		wasmPath: "./test_assets/test.wasm",
	}
	errorChan := HttpServe("localhost:8080", assets)
	<-errorChan
}

func TestGetGoVersion(t *testing.T) {
	goVersion, err := GetGoVersion()
	if err != nil {
		t.Fatal(err)
	}
	if goVersion != "1.18" {
		t.Fail()
	}
}

func TestCreatePreviewCache(t *testing.T) {
	err := CreatePreviewCache("test_assets/counter", "Counter")
	if err != nil {
		t.Fatal(err)
	}
}

func TestLockFunc(t *testing.T) {
	tests := []func() error{
		func() error {
			num := 0
			f := func() {
				time.Sleep(5 * time.Second)
				num++
			}
			lockedF := LockFunc(f)
			go lockedF()
			go lockedF()
			go lockedF()
			time.Sleep(12 * time.Second)
			if num != 2 {
				return fmt.Errorf("num != 2")
			}
			return nil
		},
		func() error {
			num := 0
			f := func() {
				time.Sleep(5 * time.Second)
				num++
			}
			lockedF := LockFunc(f)
			go lockedF()
			go lockedF()
			time.Sleep(6 * time.Second)
			go lockedF()
			time.Sleep(6 * time.Second)
			if num != 3 {
				return fmt.Errorf("num != 3")
			}
			return nil
		},
	}
	for i, test := range tests {
		err := test()
		if err != nil {
			t.Fatal(i, err)
		}
	}
}
