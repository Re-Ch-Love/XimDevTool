package preview

import (
	"bytes"
	"embed"
	"fmt"
	"github.com/fsnotify/fsnotify"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed static
var embedAssets embed.FS

type Assets struct {
	static   *embed.FS
	wasmPath string
}

func (a *Assets) Open(name string) (fs.File, error) {
	switch name {
	case ".":
		return a.static.Open("static")
	case "main.wasm":
		if a.wasmPath != "" {
			wasm, err := os.Open(a.wasmPath)
			if err != nil {
				return nil, err
			}
			return wasm, nil
		}
		return nil, fs.ErrNotExist
	}
	f, err := a.static.Open("static/" + name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func HttpServe(addr string, assets *Assets) chan error {
	handler := http.FileServer(http.FS(assets))
	http.Handle("/", handler)
	errorChan := make(chan error)
	go func() {
		err := http.ListenAndServe(addr, nil)
		if err != nil {
			errorChan <- err
		}
	}()
	return errorChan
}

// BuildProject executes `go mod tidy` and `go build -o main.wasm` in the specified directory.
func BuildProject(path string) error {
	// check go
	_, err := exec.LookPath("go") // will return an error if not found
	if err != nil {
		return err
	}
	// go mod tidy
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append([]string{"GOOS=js", "GOARCH=wasm"}, os.Environ()...)
	err = cmd.Run()
	if err != nil {
		return err
	}
	// go build
	cmd = exec.Command("go", "build", "-o", "main.wasm")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	err = cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func CreateWatcher(path string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if d != nil && d.IsDir() {
			err := watcher.Add(path)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return watcher, nil
}

// LockFunc makes the function execute only once at the same time,
// and no matter how many times it is called during execution,
// it will only execute again after the first execution.
func LockFunc(f func()) func() {
	var mutex sync.Mutex
	var needExecuteAgain bool
	return func() {
		isSucceed := mutex.TryLock()
		if !isSucceed {
			needExecuteAgain = true
			return
		}
		f()
		mutex.Unlock()
		if needExecuteAgain {
			needExecuteAgain = false
			f()
		}
	}
}

//go:embed templates
var templates embed.FS

func GetGoVersion() (string, error) {
	path, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}
	cmd := exec.Command(path, "version")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return "", err
	}
	versionWithPrefix := strings.SplitN(out.String(), " ", 4)[2]
	version := strings.TrimPrefix(versionWithPrefix, "go")
	nums := strings.SplitN(version, ".", 3)
	return nums[0] + "." + nums[1], nil
}

// CreatePreviewCache creates a complete and compilable project in `.xim/preview_cache/`
func CreatePreviewCache(componentPath string, varName string) error {
	componentPath = filepath.Clean(componentPath)
	packageName := filepath.Base(componentPath)
	// check
	stat, err := os.Stat(componentPath)
	if err != nil {
		fmt.Println("Could not get stats for", componentPath, ":", err)
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", componentPath)
	}
	// delete old files if exists
	if _, err := os.Stat(".xim/preview_cache/" + packageName); err == nil {
		err = os.RemoveAll(".xim/preview_cache/" + packageName)
		if err != nil {
			fmt.Println("Could not delete old files:", err)
			return err
		}
	}
	// make dir
	err = os.MkdirAll(".xim/preview_cache/"+packageName, 0666)
	if err != nil {
		fmt.Println("Could not create directory: .xim/preview_cache/" + packageName)
		return err
	}
	// copy files
	err = CopyDir(componentPath, ".xim/preview_cache/"+packageName)
	if err != nil {
		fmt.Println("Could not copy files:", err)
		return err
	}
	// generate main.go and go.mod
	mainGoBytes, err := GetMainGoBytes(packageName, varName)
	if err != nil {
		fmt.Println("Could not generate main.go:", err)
		return err
	}
	goVersion, err := GetGoVersion()
	if err != nil {
		fmt.Println("Could not get go version:", err)
		return err
	}
	goModBytes, err := GetGoModBytes(goVersion)
	if err != nil {
		fmt.Println("Could not generate go.mod:", err)
		return err
	}
	mainGoFile, err := os.Create(".xim/preview_cache/main.go")
	if err != nil {
		fmt.Println("Could not create main.go:", err)
		return err
	}
	written, err := mainGoFile.Write(mainGoBytes)
	if err != nil {
		fmt.Println("Could not write main.go:", err)
		return err
	}
	if written != len(mainGoBytes) {
		return fmt.Errorf("written %d bytes, but expected %d (when writing to main.go)", written, len(mainGoBytes))
	}
	goModFile, err := os.Create(".xim/preview_cache/go.mod")
	if err != nil {
		fmt.Println("Could not create go.mod:", err)
		return err
	}
	written, err = goModFile.Write(goModBytes)
	if err != nil {
		fmt.Println("Could not write go.mod:", err)
		return err
	}
	if written != len(goModBytes) {
		return fmt.Errorf("written %d bytes, but expected %d (when writing to go.mod)", written, len(goModBytes))
	}
	return nil
}

func GetMainGoBytes(packageName, varName string) ([]byte, error) {
	bs, err := templates.ReadFile("templates/main.go.tmpl")
	if err != nil {
		return nil, err
	}
	bs = bytes.ReplaceAll(bs, []byte("{{PackageName}}"), []byte(packageName))
	bs = bytes.ReplaceAll(bs, []byte("{{VarName}}"), []byte(varName))
	return bs, nil
}

func GetGoModBytes(goVersion string) ([]byte, error) {
	bs, err := templates.ReadFile("templates/go.mod.tmpl")
	if err != nil {
		return nil, err
	}
	bs = bytes.ReplaceAll(bs, []byte("{{GoVersion}}"), []byte(goVersion))
	return bs, nil
}

// CopyDir copies a directory recursively. (The specified directory itself is not included)
func CopyDir(srcPath string, dstPath string) error {
	srcPath = filepath.Clean(srcPath)
	dstPath = filepath.Clean(dstPath)
	err := filepath.WalkDir(srcPath, func(path string, d fs.DirEntry, err error) error {
		if path != srcPath {
			src, err := os.Open(path)
			defer func() {
				_ = src.Close()
			}()
			if err != nil {
				return err
			}
			dstFilePath := filepath.Join(dstPath, strings.TrimPrefix(path, srcPath))
			dst, err := os.Create(dstFilePath)
			defer func() {
				_ = dst.Close()
			}()
			if err != nil {
				return err
			}
			fmt.Println(dstFilePath)
			_, err = io.Copy(dst, src)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func Component(addr string, path string, varName string) error {
	// check path is exists and is a directory
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not stat %s: %s", path, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	assets := &Assets{
		static:   &embedAssets,
		wasmPath: ".xim/preview_cache/main.wasm",
	}
	serverErrorChan := HttpServe(addr, assets)
	watcher, err := CreateWatcher(path)
	if err != nil {
		return err
	}
	watcherDone := make(chan interface{})
	var mutex sync.Mutex
	lockedReload := LockFunc(func() {
		mutex.Lock()
		err := ReloadComponent(path, varName)
		if err != nil {
			fmt.Println("Reload error:", err)
		}
		mutex.Unlock()
	})
	// 处理watcher的协程
	go func() {
		defer close(watcherDone)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				fmt.Println("watcher event:", event.String())
				lockedReload()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("watcher error:", err)
			}
		}
	}()
	// 先加载一次
	mutex.Lock()
	err = ReloadComponent(path, varName)
	if err != nil {
		fmt.Println("Reload error:", err)
		return err
	}
	mutex.Unlock()
	// 当watcher异常关闭或http服务出错时退出程序
	select {
	case <-watcherDone:
		return fmt.Errorf("watcher unknown error")
	case err := <-serverErrorChan:
		return err
	}
}

func ReloadComponent(path string, varName string) (err error) {
	err = CreatePreviewCache(path, varName)
	if err != nil {
		fmt.Println("CreatePreviewCache error:", err)
		return
	}
	fmt.Println("building...")
	err = BuildProject(".xim/preview_cache")
	if err != nil {
		fmt.Println("build failed:", err)
		return
	}
	fmt.Println("build success")
	if err != nil {
		fmt.Println("set wasm failed:", err)
		return
	}
	return
}

func ReloadProject(path string) error {
	mainWasmPath := filepath.Join(path, "main.wasm")
	defer func() {
		err := os.Remove(mainWasmPath)
		if err != nil {
			fmt.Println("remove main.wasm failed:", err)
		}
	}()
	fmt.Println("building...")
	err := BuildProject(path)
	if err != nil {
		fmt.Println("build failed:", err)
		return err
	}
	fmt.Println("building success")
	src, err := os.Open(mainWasmPath)
	defer func() {
		_ = src.Close()
	}()
	if err != nil {
		fmt.Println("open main.wasm failed:", err)
		return err
	}
	err = os.MkdirAll(".xim/serve", 0666)
	if err != nil {
		fmt.Println("mkdir failed:", err)
		return err
	}
	dst, err := os.Create(".xim/serve/main.wasm")
	defer func() {
		_ = dst.Close()
	}()
	_, err = io.Copy(dst, src)
	if err != nil {
		fmt.Println("copy failed:", err)
		return err
	}
	return nil
}

func Project(addr string, path string) error {
	path = filepath.Clean(path)
	// check path is dir
	stat, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("could not stat %s: %s", path, err)
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	mainGoPath := filepath.Join(path, "main.go")
	// check main.go exists
	if _, err := os.Stat(mainGoPath); err != nil {
		return fmt.Errorf("%s does not exist", mainGoPath)
	}
	assets := &Assets{
		static:   &embedAssets,
		wasmPath: ".xim/serve/main.wasm",
	}
	serverErrorChan := HttpServe(addr, assets)
	watcher, err := CreateWatcher(path)
	if err != nil {
		return err
	}
	watcherDone := make(chan interface{})
	var mutex sync.Mutex
	lockedReload := LockFunc(func() {
		mutex.Lock()
		err := ReloadProject(path)
		if err != nil {
			fmt.Println("Reload error:", err)
		}
		mutex.Unlock()
	})
	// 处理watcher的协程
	go func() {
		defer close(watcherDone)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				clearName := filepath.Clean(event.Name)
				if clearName == filepath.Join(path, "main.wasm") ||
					clearName == filepath.Join(path, "go.mod") ||
					clearName == filepath.Join(path, "go.sum") ||
					clearName == filepath.Join(path, ".xim") {
					continue
				}
				fmt.Println("watcher event:", event.String())
				lockedReload()
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("watcher error:", err)
			}
		}
	}()
	// 先加载一次
	mutex.Lock()
	err = ReloadProject(path)
	if err != nil {
		fmt.Println("Reload error:", err)
		return err
	}
	mutex.Unlock()
	// 当watcher异常关闭或http服务出错时退出程序
	select {
	case <-watcherDone:
		return fmt.Errorf("watcher unknown error")
	case err := <-serverErrorChan:
		return err
	}
}
