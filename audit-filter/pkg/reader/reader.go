package reader

// TODO: redo all of this garbage

import (
	"bufio"
	"io"
	"io/ioutil"
	"log"

	"os"
	"path"
	"sync"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/audit/policy"
)

type Line struct {
	Index uint64
	Data []byte
}

type RotatingReader struct {
	*io.PipeReader
	Path string
	file *os.File
	*fsnotify.Watcher
	inode uint64
	sync.WaitGroup
	rotated chan struct{}
	updated chan struct{}
	writer  *io.PipeWriter
}

func NewRotatingReader(follow bool, filepath string) *RotatingReader {
	f := &RotatingReader{}
	f.PipeReader, f.writer = io.Pipe()
//	if follow {
		f.monitorPath(follow, filepath)
//	}
	go func() {
		defer func() {
			f.writer.Close()
//			close(f.Events)
			<-f.updated
			close(f.updated)
		}()
		for {
		read_file:
			for {
				f.Wait()
				reader := io.TeeReader(f.file, f.writer)
				io.Copy(ioutil.Discard, reader)
				if !follow {
					return
				}
				select {
				case <-f.rotated:
					go io.Copy(ioutil.Discard, reader)
					break read_file
				case <-f.updated:
					for {
						n, err := io.Copy(ioutil.Discard, reader)
						if n == 0 || err != nil {
							break
						}
					}
				}
			}
		}
	}()
	return f
}

func (r *RotatingReader) monitorPath(follow bool, filepath string) {
	r.Path = filepath
	fd, err := os.Open(filepath)
	if err != nil {
		r.WaitGroup.Add(1)
	}
	r.file = fd
	r.Watcher, err = fsnotify.NewWatcher()
	r.updated = make(chan struct{})
	r.rotated = make(chan struct{})
	r.Watcher.Add(filepath)
	r.Watcher.Add(path.Dir(filepath))
	go func() {
		for {
			<-r.Events
			if r.file == nil {
				r.file, err = os.Open(filepath)
				if err != nil {
					continue
				}
				r.inode = GetInode(r.file)
				r.Done()
			}
			if !follow {
				return
			}
			r.updated <- struct{}{}
			if r.file != nil && r.hasRotated() {
				r.file = nil
				if !follow {
					return
				}
				r.rotated <- struct{}{}
				r.WaitGroup.Add(1)
			}
		}
	}()
}

func (f *RotatingReader) hasRotated() bool {
	if f.inode == 0 {
		f.inode = GetInode(f.file)
	}
	return f.inode != GetInode(f.Path)
}

func GetInode(file interface{}) uint64 {
	var fi os.FileInfo
	var err error
	switch file.(type) {
	case *os.File:
		fi, err = file.(*os.File).Stat()
	default:
		fi, err = os.Stat(file.(string))
	}
	if err != nil {
		return 0
	}
	return fi.Sys().(*syscall.Stat_t).Ino
}

func WatchPolicyPath(filepath string, p *audit.Policy) {
	var wd *fsnotify.Watcher
	var err error
	for {
		wd, err = fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}
		wd.Add(filepath)
		wd.Add(path.Dir(filepath))
		wd.Add(path.Dir(path.Dir(filepath))) // for k8s
		for {
			select {
			case ev := <-wd.Events:
				if ev.Op&(fsnotify.Rename|fsnotify.Remove|fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				newPolicy, err := policy.LoadPolicyFromFile(filepath)
				if err != nil {
					continue
				}
				*p = *newPolicy
				log.Println("policy has been reloaded")
			case err := <-wd.Errors:
				log.Println(err)
			}
			break
		}
	}
}


func ReadFiles(follow bool, paths ...string) <-chan Line {

	var wg sync.WaitGroup
	var m sync.Mutex
	_index := uint64(0)
	index := func() uint64 {
		defer m.Unlock()
		m.Lock()
		_index += 1
		return _index
	}

	output := make(chan Line)

	mux := func(f *RotatingReader, output chan Line) {
		defer wg.Done()
		reader := bufio.NewReaderSize(f, 102400)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if !follow {
					return
				}
				continue
			}
			output <- Line{index(), line[:]}
		}
	}

	wg.Add(len(paths))
	for _, path := range paths {
		go mux(NewRotatingReader(follow, path), output)
	}

	go func() {
		defer close(output)
		wg.Wait()
	}()

	return output
}
