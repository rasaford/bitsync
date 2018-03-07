package file

import (
	"os"
	"reflect"
	"testing"

	"github.com/rasaford/bitsync/torrent/hash"
)

const testDir = "../../syncTest"

func TestOSFileSystem_Open(t *testing.T) {
	fs, _ := NewOSFileSystem(testDir)
	type args struct {
		path []string
	}
	tests := []struct {
		name    string
		fs      FileSystem
		args    args
		want    string
		wantErr bool
	}{
		{
			"open non existent file",
			fs,
			args{[]string{"nonExistentFile.txt"}},
			"",
			true,
		},
		{
			"open non existent file",
			fs,
			args{[]string{"001LossofSleepDestroysAbility.png"}},
			string([]byte{218, 57, 163, 238, 94, 107, 75, 13,
				50, 85, 191, 239, 149, 96, 24, 144, 175, 216, 7, 9,
			}),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fs.Open(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("OSFileSystem.Open() error = %v, wantErr %v", err, tt.wantErr)
				return
			} else if err != nil {
				return
			}
			if got != nil {
				defer got.Close()
			}
			if hash := hash.SHA1(got); hash != tt.want {
				t.Errorf("OSFileSystem.Open() = %v, want %v", hash, tt.want)
			}
		})
	}
}

func TestOSFileSystem_Stat(t *testing.T) {
	fs, _ := NewOSFileSystem(testDir)
	type args struct {
		path []string
	}
	tests := []struct {
		name    string
		fs      FileSystem
		args    args
		want    os.FileInfo
		wantErr bool
	}{
		{
			"stat file nonexistent",
			fs,
			args{[]string{"nonexistentFile.png"}},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fs.Stat(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("OSFileSystem.Stat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OSFileSystem.Stat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOSFileSystem_List(t *testing.T) {
	fs, _ := NewOSFileSystem(testDir)
	type args struct {
		path []string
	}
	tests := []struct {
		name    string
		fs      FileSystem
		args    args
		want    []string
		wantErr bool
	}{
		{
			"list nonexistent files",
			fs,
			args{[]string{"non", "existent", "folder"}},
			nil,
			true,
		},
		{
			"list test files",
			fs,
			args{[]string{}},
			[]string{
				"001LossofSleepDestroysAbility.png",
				"002CaffeineHelps.png",
				"003Effectofselfinstruction.png",
				"files.bitsync",
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fs.List(tt.args.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("OSFileSystem.List() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("OSFileSystem.List() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewOSFileSystem(t *testing.T) {
	type args struct {
		directory string
	}
	tests := []struct {
		name    string
		args    args
		want    FileSystem
		wantErr bool
	}{
		{
			"empty dir",
			args{""},
			nil,
			true,
		},
		{
			"invalid path",
			args{"/\\/\\"},
			nil,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewOSFileSystem(tt.args.directory)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOSFileSystem() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewOSFileSystem() = %v, want %v", got, tt.want)
			}
		})
	}
}
