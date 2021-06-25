/*
Copyright 2021 The cert-manager Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/cert-manager/csi-lib/metadata"
)

func TestFilesystem_ReadFile(t *testing.T) {
	backend := &Filesystem{
		fs: fstest.MapFS{
			"inmemfs/fake-volume/data/file": &fstest.MapFile{Data: []byte("hello world")},
		},
	}

	d, err := backend.ReadFile("fake-volume", "file")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if string(d) != "hello world" {
		t.Errorf("expected contents 'hello world' but got: %v", string(d))
	}
}

func TestFilesystem_ReadFile_NotFound(t *testing.T) {
	backend := &Filesystem{
		fs: fstest.MapFS{
			"inmemfs/fake-volume/data/file": &fstest.MapFile{Data: []byte("hello world")},
		},
	}

	_, err := backend.ReadFile("fake-volume", "file2")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected %v but got: %v", ErrNotFound, err)
	}
}

func TestFilesystem_MetadataForVolume_NotFound(t *testing.T) {
	backend := &Filesystem{
		fs: fstest.MapFS{},
	}

	_, err := backend.ReadMetadata("fake-volume")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected %v but got: %v", ErrNotFound, err)
	}
}

func TestFilesystem_MetadataForVolume_InvalidJSON(t *testing.T) {
	backend := &Filesystem{
		fs: fstest.MapFS{
			"inmemfs/fake-volume/metadata.json": &fstest.MapFile{Data: []byte("{")},
		},
	}

	_, err := backend.ReadMetadata("fake-volume")
	if !errors.Is(err, ErrInvalidJSON) {
		t.Errorf("expected %v but got: %v", ErrInvalidJSON, err)
	}
}

func TestFilesystem_MetadataForVolume(t *testing.T) {
	backend := &Filesystem{
		fs: fstest.MapFS{
			"inmemfs/fake-volume/metadata.json": &fstest.MapFile{Data: []byte(`{"volumeID": "fake-volume", "targetPath": "/fake-volume", "volumeContext": {"a": "b"}}`)},
		},
	}

	meta, err := backend.ReadMetadata("fake-volume")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(meta, metadata.Metadata{
		VolumeID:      "fake-volume",
		TargetPath:    "/fake-volume",
		VolumeContext: map[string]string{"a": "b"},
	}) {
		t.Errorf("unexpected metadata: %#v", meta)
	}
}

func TestFilesystem_ListVolumes(t *testing.T) {
	backend := &Filesystem{
		fs: fstest.MapFS{
			"inmemfs/fake-volume/metadata.json": &fstest.MapFile{Data: []byte{}},
		},
	}

	vols, err := backend.ListVolumes()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(vols) != 1 {
		t.Errorf("expected 1 volume to be returned but got: %+v", vols)
	}
	if vols[0] != "fake-volume" {
		t.Errorf("expected only entry to be 'fake-volume' but got: %s", vols[0])
	}
}

func Test_fsGroupForMetadata(t *testing.T) {
	intPtr := func(i int64) *int64 {
		return &i
	}
	strPtr := func(s string) *string {
		return &s
	}

	tests := map[string]struct {
		fixedFSGroup              *int64
		fsGroupVolumeAttributeKey *string
		volumeContext             map[string]string

		expGID *int64
		expErr bool
	}{
		"FixedFSGroup=nil FSGroupVolumeAttributeKey=nil, should return nil gid": {
			fixedFSGroup:              nil,
			fsGroupVolumeAttributeKey: nil,
			volumeContext:             map[string]string{},
			expGID:                    nil,
			expErr:                    false,
		},
		"FixedFSGroup=10 FSGroupVolumeAttributeKey=nil, should return 10": {
			fixedFSGroup:              intPtr(10),
			fsGroupVolumeAttributeKey: nil,
			volumeContext:             map[string]string{},
			expGID:                    intPtr(10),
			expErr:                    false,
		},
		"FixedFSGroup=nil FSGroupVolumeAttributeKey=defined but not present in context, should return nil": {
			fixedFSGroup:              nil,
			fsGroupVolumeAttributeKey: strPtr("fs-gid"),
			volumeContext:             map[string]string{},
			expGID:                    nil,
			expErr:                    false,
		},
		"FixedFSGroup=nil FSGroupVolumeAttributeKey=defined and present in context, should return 20": {
			fixedFSGroup:              nil,
			fsGroupVolumeAttributeKey: strPtr("fs-gid"),
			volumeContext: map[string]string{
				"fs-gid": "20",
			},
			expGID: intPtr(20),
			expErr: false,
		},
		"FixedFSGroup=nil FSGroupVolumeAttributeKey=defined and present in context but with bad value, should return error": {
			fixedFSGroup:              nil,
			fsGroupVolumeAttributeKey: strPtr("fs-gid"),
			volumeContext: map[string]string{
				"fs-gid": "bad-value",
			},
			expGID: nil,
			expErr: true,
		},
		"FixedFSGroup=10 FSGroupVolumeAttributeKey=defined and present in context, should return superseding FixedFSGroup (10)": {
			fixedFSGroup:              intPtr(10),
			fsGroupVolumeAttributeKey: strPtr("fs-gid"),
			volumeContext: map[string]string{
				"fs-gid": "20",
			},
			expGID: intPtr(10),
			expErr: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := Filesystem{
				FixedFSGroup:              test.fixedFSGroup,
				FSGroupVolumeAttributeKey: test.fsGroupVolumeAttributeKey,
			}

			gid, err := f.fsGroupForMetadata(metadata.Metadata{
				VolumeContext: test.volumeContext,
			})

			if (err != nil) != test.expErr {
				t.Errorf("unexpected error, exp=%t got=%v", test.expErr, err)
			}

			if !reflect.DeepEqual(gid, test.expGID) {
				t.Errorf("unexpected gid, exp=%v got=%v", test.expGID, gid)
			}
		})
	}
}
