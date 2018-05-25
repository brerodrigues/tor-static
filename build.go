package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

var verbose bool
var folders = []string{"openssl", "libevent", "zlib", "xz", "tor"}
var absCurrDir = getAbsCurrDir()

func main() {
	flag.BoolVar(&verbose, "verbose", false, "Whether to show command output")
	flag.Parse()
	if len(flag.Args()) != 1 {
		log.Fatal("Missing command. Can be build-all, build-<folder>, clean-all, or clean-<folder>")
	}
	if err := run(flag.Args()[0]); err != nil {
		log.Fatal(err)
	}
}

func run(cmd string) error {
	if err := validateEnvironment(); err != nil {
		return err
	}
	if strings.HasPrefix(cmd, "build-") {
		return build(cmd[6:])
	} else if strings.HasPrefix(cmd, "clean-") {
		return clean(cmd[6:])
	}
	return fmt.Errorf("Invalid command: %v. Should be build-all, build-<folder>, clean-all, or clean-<folder>", cmd)
}

func getAbsCurrDir() string {
	var err error
	absCurrDir, err := filepath.Abs(".")
	if err != nil {
		panic(err)
	}
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(absCurrDir)
		absCurrDir = "/" + strings.TrimSuffix(volume, ":") + "/" + filepath.ToSlash(absCurrDir[len(volume)+1:])
	}
	return absCurrDir
}

func validateEnvironment() error {
	// Make sure all the folders are there
	for _, folder := range folders {
		if info, err := os.Stat(folder); err != nil || !info.IsDir() {
			return fmt.Errorf("%v is not a dir", folder)
		}
	}
	switch runtime.GOOS {
	// On windows, have to verify MinGW
	case "windows":
		// Confirm it is MinGW 64
		if byts, err := exec.Command("uname", "-a").CombinedOutput(); err != nil {
			return fmt.Errorf("This has to be run in a MSYS or MinGW shell")
		} else if !bytes.HasPrefix(byts, []byte("MINGW64")) {
			return fmt.Errorf("This has to be run in a MSYS or MinGW64 shell")
		}
		// Have to have downgraded specific packages (TODO: fix when MinGW is fixed)
		confirmPackage := func(pkg string, vers string) error {
			if byts, err := exec.Command("pacman", "-Q", pkg).CombinedOutput(); err != nil {
				return fmt.Errorf("Failed running pacman to check packages: %v", err)
			} else if !bytes.Equal([]byte(pkg+" "+vers), bytes.TrimSpace(byts)) {
				return fmt.Errorf("Expected '%v %v' got '%v'. Must downgrade some packages for MinGW to work. Use:\n"+
					"    pacman -U /var/cache/pacman/pkg/mingw-w64-x86_64-crt-git-5.0.0.4745.d2384c2-1-any.pkg.tar.xz\n"+
					"    pacman -U /var/cache/pacman/pkg/mingw-w64-x86_64-headers-git-5.0.0.4747.0f8f626-1-any.pkg.tar.xz\n"+
					"    pacman -U /var/cache/pacman/pkg/mingw-w64-x86_64-winpthreads-git-5.0.0.4741.2c8939a-1-any.pkg.tar.xz\n"+
					"    pacman -U /var/cache/pacman/pkg/mingw-w64-x86_64-libwinpthread-git-5.0.0.4741.2c8939a-1-any.pkg.tar.xz",
					pkg, vers, bytes.TrimSpace(byts))
			}
			return nil
		}
		err := confirmPackage("mingw-w64-x86_64-crt-git", "5.0.0.4745.d2384c2-1")
		if err == nil {
			err = confirmPackage("mingw-w64-x86_64-headers-git", "5.0.0.4747.0f8f626-1")
		}
		if err == nil {
			err = confirmPackage("mingw-w64-x86_64-winpthreads-git", "5.0.0.4741.2c8939a-1")
		}
		if err == nil {
			err = confirmPackage("mingw-w64-x86_64-libwinpthread-git", "5.0.0.4741.2c8939a-1")
		}
		if err != nil {
			return err
		}
	case "linux":
		// Make sure it's not MinGW
		if byts, err := exec.Command("uname", "-a").CombinedOutput(); err != nil {
			return fmt.Errorf("Failed running uname -a")
		} else if bytes.HasPrefix(byts, []byte("MINGW")) {
			return fmt.Errorf("MinGW should not use Linux Go binary, but instead a Windows go.exe to run the build")
		}
	}
	return nil
}

func build(folder string) error {
	log.Printf("*** Building %v ***", folder)
	defer log.Printf("*** Done building %v ***", folder)
	pwd := absCurrDir + "/" + folder
	switch folder {
	case "all":
		for _, subFolder := range folders {
			if err := clean(subFolder); err != nil {
				return err
			}
		}
		return nil
	case "openssl":
		cmds := [][]string{
			{"sh", "./config", "--prefix=" + pwd + "/dist", "no-shared", "no-dso", "no-zlib"},
			{"make", "depend"},
			{"make"},
			{"make", "install"},
		}
		if runtime.GOOS == "windows" {
			cmds[0] = append(cmds[0], "mingw64")
			cmds[0][1] = "./Configure"
		}
		return runCmds(folder, nil, cmds)
	case "libevent":
		return runCmds(folder, nil, [][]string{
			{"sh", "-l", "./autogen.sh"},
			{"sh", "./configure", "--prefix=" + pwd + "/dist",
				"--disable-shared", "--enable-static", "--with-pic"},
			{"make"},
			{"make", "install"},
		})
	case "zlib":
		var env []string
		cmds := [][]string{{"sh", "./configure", "--prefix=" + pwd + "/dist"}, {"make"}, {"make", "install"}}
		if runtime.GOOS == "windows" {
			env = []string{"PREFIX=" + pwd + "/dist", "BINARY_PATH=" + pwd + "/dist/bin",
				"INCLUDE_PATH=" + pwd + "/dist/include", "LIBRARY_PATH=" + pwd + "/dist/lib"}
			cmds = [][]string{{"make", "-fwin32/Makefile.gcc"}, {"make", "install", "-fwin32/Makefile.gcc"}}
		}
		return runCmds(folder, env, cmds)
	case "xz":
		return runCmds(folder, nil, [][]string{
			{"sh", "-l", "./autogen.sh"},
			{"sh", "./configure", "--prefix=" + pwd + "/dist", "--disable-shared", "--enable-static",
				"--disable-doc", "--disable-scripts", "--disable-xz", "--disable-xzdec", "--disable-lzmadec",
				"--disable-lzmainfo", "--disable-lzma-links"},
			{"make"},
			{"make", "install"},
		})
	case "tor":
		// We have to make a symlink from zlib to openssl
		if _, err := os.Stat("openssl/dist/lib/libz.a"); os.IsNotExist(err) {
			err = runCmd("", nil, "ln", "-s", pwd+"/../zlib/dist/lib/libz.a", pwd+"/../openssl/dist/lib/libz.a")
			if err != nil {
				return fmt.Errorf("Unable to make symlink: %v", err)
			}
		}
		var env []string
		if runtime.GOOS == "windows" {
			env = []string{"LIBS=-lcrypt32"}
		}
		return runCmds(folder, env, [][]string{
			{"sh", "-l", "./autogen.sh"},
			{"sh", "./configure", "--prefix=" + pwd + "/dist", "--disable-gcc-hardening", "--enable-static-tor",
				"--enable-static-libevent", "--with-libevent-dir=" + pwd + "/../libevent/dist", "--enable-static-openssl",
				"--with-openssl-dir=" + pwd + "/../openssl/dist", "--enable-static-zlib",
				"--with-zlib-dir=" + pwd + "/../openssl/dist", "--disable-system-torrc", "--disable-asciidoc"},
			{"make"},
			{"make", "install"},
		})
	default:
		return fmt.Errorf("Unrecognized folder: %v", folder)
	}
}

func clean(folder string) (err error) {
	log.Printf("*** Cleaning %v ***", folder)
	defer log.Printf("*** Done cleaning %v ***", folder)
	switch folder {
	case "all":
		for _, subFolder := range folders {
			if err = clean(subFolder); err != nil {
				break
			}
		}
	default:
		args := []string{"clean"}
		env := []string{}
		makefile := "Makefile"
		switch folder {
		// OpenSSL needs to have the dist folder removed first
		case "openssl":
			if err := os.RemoveAll("openssl/dist/lib"); err != nil {
				return fmt.Errorf("Unable to remove openssl/dist/lib: %v", err)
			}
		// Zlib needs to have a prefix and needs a special windows makefile
		case "zlib":
			env = append(env, "PREFIX="+absCurrDir+"/zlib/dist")
			if runtime.GOOS == "windows" {
				makefile = "win32/Makefile.gcc"
				args = append(args, "-fwin32/Makefile.gcc")
			}
		}
		if dir, err := os.Stat(folder); err != nil || !dir.IsDir() {
			return fmt.Errorf("%v is not a directory", folder)
		} else if _, err := os.Stat(path.Join(folder, makefile)); os.IsNotExist(err) {
			log.Printf("Skipping clean, makefile not present")
			return nil
		}
		err = runCmd(folder, env, "make", args...)
	}
	return err
}

func runCmds(folder string, env []string, cmdsAndArgs [][]string) error {
	for _, cmdAndArgs := range cmdsAndArgs {
		if err := runCmd(folder, env, cmdAndArgs[0], cmdAndArgs[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func runCmd(folder string, env []string, cmd string, args ...string) error {
	log.Printf("Running in folder %v: %v %v", folder, cmd, strings.Join(args, " "))
	c := exec.Command(cmd, args...)
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	c.Dir = folder
	if verbose {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	}
	return c.Run()
}