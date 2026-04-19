package main

import (
	"archive/zip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func checkAdmin() bool {
	if runtime.GOOS == "windows" {
		_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
		return err == nil
	}
	return os.Geteuid() == 0
}

func elevateAdmin() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("elevation not implemented for this OS")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// Using PowerShell to restart as admin
	cmd := exec.Command("powershell", "Start-Process", exe, "-Verb", "RunAs")
	return cmd.Start()
}

func openTerminal(path string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "start", "cmd.exe", "/k", "cd", "/d", path)
	} else {
		cmd = exec.Command("x-terminal-emulator", "--working-directory", path)
	}
	return cmd.Run()
}

func openExplorer(path string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("explorer", path)
	} else {
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Run()
}

func getDrives() ([]FileItem, error) {
	if runtime.GOOS != "windows" {
		return []FileItem{{Name: "/", IsDir: true}}, nil
	}
	var drives []FileItem
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		d := string(drive) + ":\\"
		if _, err := os.Stat(d); err == nil {
			drives = append(drives, FileItem{
				Name:  d,
				IsDir: true,
			})
		}
	}
	return drives, nil
}

func listFiles(path string) ([]FileItem, error) {
	if path == "This PC" {
		return getDrives()
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var items []FileItem
	for _, entry := range entries {
		info, _ := entry.Info()
		items = append(items, FileItem{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
			Mode:  info.Mode(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir && !items[j].IsDir { return true }
		if !items[i].IsDir && items[j].IsDir { return false }
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func encryptFile(filename string, password string) error {
	plaintext, err := os.ReadFile(filename)
	if err != nil { return err }
	key := sha256.Sum256([]byte(password))
	block, err := aes.NewCipher(key[:])
	if err != nil { return err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return err }
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil { return err }
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return os.WriteFile(filename+".enc", ciphertext, 0644)
}

func decryptFile(filename string, password string) error {
	ciphertext, err := os.ReadFile(filename)
	if err != nil { return err }
	key := sha256.Sum256([]byte(password))
	block, err := aes.NewCipher(key[:])
	if err != nil { return err }
	gcm, err := cipher.NewGCM(block)
	if err != nil { return err }
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize { return fmt.Errorf("invalid ciphertext") }
	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil { return err }
	origName := filename
	if strings.HasSuffix(filename, ".enc") {
		origName = filename[:len(filename)-4]
	} else {
		origName = filename + ".dec"
	}
	return os.WriteFile(origName, plaintext, 0644)
}

func zipFiles(source, target string) error {
	zipFile, err := os.Create(target)
	if err != nil { return err }
	defer zipFile.Close()
	archive := zip.NewWriter(zipFile)
	defer archive.Close()
	info, err := os.Stat(source)
	if err != nil { return err }
	var baseDir string
	if info.IsDir() { baseDir = filepath.Base(source) }
	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil { return err }
		header, err := zip.FileInfoHeader(info)
		if err != nil { return err }
		if baseDir != "" {
			header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
		}
		if info.IsDir() { header.Name += "/" } else { header.Method = zip.Deflate }
		writer, err := archive.CreateHeader(header)
		if err != nil { return err }
		if info.IsDir() { return nil }
		file, err := os.Open(path)
		if err != nil { return err }
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
	return nil
}

func unzipArchive(source, target string) error {
	reader, err := zip.OpenReader(source)
	if err != nil { return err }
	defer reader.Close()
	for _, f := range reader.File {
		fpath := filepath.Join(target, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil { return err }
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil { return err }
		rc, err := f.Open()
		if err != nil { return err }
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil { return err }
	}
	return nil
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil { return err }
	if info.IsDir() {
		return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil { return err }
			rel, err := filepath.Rel(src, path)
			if err != nil { return err }
			newDst := filepath.Join(dst, rel)
			if info.IsDir() { return os.MkdirAll(newDst, info.Mode()) }
			return copySingleFile(path, newDst)
		})
	}
	return copySingleFile(src, dst)
}

func copySingleFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil { return err }
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil { return err }
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
