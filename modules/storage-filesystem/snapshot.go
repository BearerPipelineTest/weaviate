//                           _       _
// __      _____  __ ___   ___  __ _| |_ ___
// \ \ /\ / / _ \/ _` \ \ / / |/ _` | __/ _ \
//  \ V  V /  __/ (_| |\ V /| | (_| | ||  __/
//   \_/\_/ \___|\__,_| \_/ |_|\__,_|\__\___|
//
//  Copyright © 2016 - 2022 SeMI Technologies B.V. All rights reserved.
//
//  CONTACT: hello@semi.technology
//

package modstgfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/semi-technologies/weaviate/entities/backup"
	"github.com/semi-technologies/weaviate/usecases/monitoring"
)

func (m *StorageFileSystemModule) StoreSnapshot(ctx context.Context, snapshot *backup.Snapshot) error {
	timer := prometheus.NewTimer(monitoring.GetMetrics().SnapshotStoreDurations.WithLabelValues("filesystem", snapshot.ClassName))
	defer timer.ObserveDuration()
	if err := ctx.Err(); err != nil {
		return backup.NewErrContextExpired(
			errors.Wrap(err, "store snapshot aborted"))
	}

	dstSnapshotPath, err := m.createBackupDir(snapshot)
	if err != nil {
		return err
	}

	for _, file := range snapshot.Files {
		if err := ctx.Err(); err != nil {
			return backup.NewErrContextExpired(
				errors.Wrap(err, "store snapshot aborted"))
		}
		if err := m.copyFile(dstSnapshotPath, m.dataPath, file.Path); err != nil {
			return err
		}

		destPath := m.makeSnapshotFilePath(snapshot.ClassName, snapshot.ID, file.Path)
		// Get size of file
		fileInfo, err := os.Stat(destPath)
		if err != nil {
			return errors.Errorf("Unable to get size of file %v", destPath)
		}
		monitoring.GetMetrics().SnapshotStoreDataTransferred.WithLabelValues("filesystem", snapshot.ClassName).Add(float64(fileInfo.Size()))
	}

	if err := m.saveMeta(snapshot); err != nil {
		return err
	}

	return nil
}

func (m *StorageFileSystemModule) RestoreSnapshot(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error) {
	timer := prometheus.NewTimer(monitoring.GetMetrics().SnapshotRestoreFromStorageDurations.WithLabelValues("filesystem", className))
	defer timer.ObserveDuration()
	snapshot, err := m.loadSnapshotMeta(ctx, className, snapshotID)
	if err != nil {
		return nil, errors.Wrap(err, "restore snapshot")
	}

	for _, file := range snapshot.Files {
		destPath := m.makeSnapshotFilePath(className, snapshotID, file.Path)

		if err := ctx.Err(); err != nil {
			return nil, errors.Wrap(err, "restore snapshot aborted, system might be in an invalid state")
		}
		if err := m.copyFile(m.dataPath, m.makeSnapshotDirPath(className, snapshotID), file.Path); err != nil {
			return nil, errors.Wrapf(err, "restore snapshot aborted, system might be in an invalid state: file %v", file.Path)
		}
		if err := m.copyFile(m.dataPath, m.makeSnapshotDirPath(className, snapshotID), file.Path); err != nil {
			return nil, errors.Wrapf(err, "restore snapshot aborted, system might be in an invalid state: file %v", file.Path)
		}

		// Get size of file
		fileInfo, err := os.Stat(destPath)
		if err != nil {
			return nil, errors.Errorf("Unable to get size of file %v", destPath)
		}
		monitoring.GetMetrics().SnapshotRestoreDataTransferred.WithLabelValues("filesystem", className).Add(float64(fileInfo.Size()))

	}

	return snapshot, nil
}

func (m *StorageFileSystemModule) loadSnapshotMeta(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, errors.Wrap(err, "load snapshot meta")
	}

	metaPath := m.makeMetaFilePath(className, snapshotID)

	if _, err := os.Stat(metaPath); errors.Is(err, os.ErrNotExist) {
		return nil, backup.NewErrNotFound(err)
	} else if err != nil {
		return nil, backup.NewErrInternal(err)
	}

	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, backup.NewErrInternal(
			errors.Wrapf(err, "read snapshot meta file '%v'", metaPath))
	}

	var snapshot backup.Snapshot
	if err := json.Unmarshal(metaData, &snapshot); err != nil {
		return nil, backup.NewErrInternal(
			errors.Wrap(err, "unmarshal snapshot meta"))
	}

	return &snapshot, nil
}

func (m *StorageFileSystemModule) GetMeta(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error) {
	return m.loadSnapshotMeta(ctx, className, snapshotID)
}

func (m *StorageFileSystemModule) InitSnapshot(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error) {
	snapshot := backup.NewSnapshot(className, snapshotID, time.Now())
	snapshot.Status = string(backup.CreateStarted)

	if err := m.saveMeta(snapshot); err != nil {
		return nil, backup.NewErrInternal(errors.Wrap(err, "init snapshot meta"))
	}

	return snapshot, nil
}

func (m *StorageFileSystemModule) SetMetaStatus(ctx context.Context, className, snapshotID, status string) error {
	snapshot, err := m.loadSnapshotMeta(ctx, className, snapshotID)
	if err != nil {
		return backup.NewErrInternal(errors.Wrap(err, "set meta status"))
	}

	snapshot.Status = string(status)

	if err := m.saveMeta(snapshot); err != nil {
		return backup.NewErrInternal(errors.Wrap(err, "set meta status"))
	}

	return nil
}

func (m *StorageFileSystemModule) SetMetaError(ctx context.Context, className, snapshotID string, snapErr error) error {
	snapshot, err := m.loadSnapshotMeta(ctx, className, snapshotID)
	if err != nil {
		return backup.NewErrInternal(errors.Wrap(err, "set meta error"))
	}

	snapshot.Status = string(backup.CreateFailed)
	snapshot.Error = snapErr.Error()

	if err := m.saveMeta(snapshot); err != nil {
		return backup.NewErrInternal(errors.Wrap(err, "set meta error"))
	}

	return nil
}

func (m *StorageFileSystemModule) initSnapshotStorage(ctx context.Context, snapshotsPath string) error {
	if snapshotsPath == "" {
		return fmt.Errorf("empty snapshots path provided")
	}
	snapshotsPath = filepath.Clean(snapshotsPath)
	if !filepath.IsAbs(snapshotsPath) {
		return fmt.Errorf("relative snapshots path provided")
	}
	if err := m.createBackupsDir(snapshotsPath); err != nil {
		return errors.Wrap(err, "invalid snapshots path provided")
	}
	m.snapshotsPath = snapshotsPath

	return nil
}

func (m *StorageFileSystemModule) createBackupsDir(snapshotsPath string) error {
	if err := os.MkdirAll(snapshotsPath, os.ModePerm); err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "create_snapshots_dir").
			WithError(err).
			Errorf("failed creating snapshots directory %v", snapshotsPath)
		return backup.NewErrInternal(errors.Wrap(err, "make snapshot dir"))
	}
	return nil
}

func (m *StorageFileSystemModule) createBackupDir(snapshot *backup.Snapshot) (snapshotPath string, err error) {
	snapshotPath = m.makeSnapshotDirPath(snapshot.ClassName, snapshot.ID)
	return snapshotPath, m.createBackupsDir(snapshotPath)
}

func (m *StorageFileSystemModule) copyFile(dstSnapshotPath, srcBasePath, srcRelPath string) error {
	srcAbsPath := filepath.Join(srcBasePath, srcRelPath)
	dstAbsPath := filepath.Join(dstSnapshotPath, srcRelPath)

	src, err := os.Open(srcAbsPath)
	if err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "copy_file").
			WithError(err).
			Errorf("failed opening source file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "open snapshot source file '%v'", srcRelPath))
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstAbsPath), os.ModePerm); err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "copy_file").
			WithError(err).
			Errorf("failed creating destication dir for file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "create snapshot destination dir for file '%v'", srcRelPath))
	}
	dst, err := os.Create(dstAbsPath)
	if err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "copy_file").
			WithError(err).
			Errorf("failed creating destication file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "create snapshot destination file '%v'", srcRelPath))
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "copy_file").
			WithError(err).
			Errorf("failed copying snapshot file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "copy snapshot file '%v'", srcRelPath))
	}

	return nil
}

func (m *StorageFileSystemModule) saveMeta(snapshot *backup.Snapshot) error {
	content, err := json.Marshal(snapshot)
	if err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "save_meta").
			WithField("snapshot_classname", snapshot.ClassName).
			WithField("snapshot_id", snapshot.ID).
			WithError(err).
			Errorf("failed creating meta file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "create meta file for snapshot '%v'", snapshot.ID))
	}

	metaFile := m.makeMetaFilePath(snapshot.ClassName, snapshot.ID)
	metaDir := path.Dir(metaFile)

	if err := os.MkdirAll(metaDir, os.ModePerm); err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "save_meta").
			WithField("snapshot_classname", snapshot.ClassName).
			WithField("snapshot_id", snapshot.ID).
			WithError(err).
			Errorf("failed creating meta file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "create meta file for snapshot '%v'", snapshot.ID))
	}

	// We first need to write to a temporary file because there might be a status request
	// during the write operation which will try to read the snapshot.json file
	// and if this request will occur during the WriteFile operation then this
	// status request may encounter an empty file (because it's being overwritten)
	// In order to solve this problem we first save a new snapshot.json file to a temporary
	// file, and then rename it, that way we always have a snapshot.json file present
	tmpMetaFile := fmt.Sprintf("%s%s", metaFile, ".tmp")
	if err := os.WriteFile(tmpMetaFile, content, os.ModePerm); err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "save_meta").
			WithField("snapshot_classname", snapshot.ClassName).
			WithField("snapshot_id", snapshot.ID).
			WithError(err).
			Errorf("failed creating meta file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "create temporary meta file for snapshot %v", snapshot.ID))
	}

	if os.Rename(tmpMetaFile, metaFile); err != nil {
		m.logger.WithField("module", m.Name()).
			WithField("action", "rename_meta").
			WithField("snapshot_classname", snapshot.ClassName).
			WithField("snapshot_id", snapshot.ID).
			WithError(err).
			Errorf("failed to rename meta file")
		return backup.NewErrInternal(
			errors.Wrapf(err, "rename temporary meta file for snapshot %v", snapshot.ID))
	}

	return nil
}
