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

package modulecapabilities

import (
	"context"

	"github.com/semi-technologies/weaviate/entities/backup"
)

type SnapshotStorage interface {
	StoreSnapshot(ctx context.Context, snapshot *backup.Snapshot) error
	RestoreSnapshot(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error)

	InitSnapshot(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error)
	GetMeta(ctx context.Context, className, snapshotID string) (*backup.Snapshot, error)
	SetMetaStatus(ctx context.Context, className, snapshotID, status string) error
	SetMetaError(ctx context.Context, className, snapshotID string, err error) error
	DestinationPath(className, snapshotID string) string
}
