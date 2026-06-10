// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package app

import (
	"context"
	"fmt"

	"rims-go/internal/modules/document"
	"rims-go/internal/modules/file"
	"rims-go/internal/modules/warehouse"
)

type fileAccessChecker struct {
	docRepo document.DocumentRepository
	whRepo  warehouse.UserWarehouseRepository
}

func (c fileAccessChecker) CanAccessFile(ctx context.Context, actor file.FileActor, f *file.FileAttachment, action file.FileAction) (bool, error) {
	if action == file.FileActionDelete {
		return actor.IsAdmin || f.CreatedBy == actor.UserID, nil
	}
	if action != file.FileActionCreate && (actor.IsAdmin || f.CreatedBy == actor.UserID) {
		return true, nil
	}
	if f.BusinessID == nil {
		return false, nil
	}

	switch f.BusinessType {
	case file.BusinessTypeDocAttachment:
		if c.docRepo == nil {
			return false, fmt.Errorf("file acl: document repository is nil")
		}
		if c.whRepo == nil {
			return false, fmt.Errorf("file acl: user warehouse repository is nil")
		}
		doc, err := c.docRepo.GetByID(ctx, *f.BusinessID)
		if err != nil {
			return false, err
		}
		return c.whRepo.HasAccess(ctx, actor.UserID, doc.WarehouseID)
	case file.BusinessTypeProductImage:
		if action == file.FileActionCreate {
			return actor.IsAdmin, nil
		}
		return action == file.FileActionRead, nil
	default:
		return false, nil
	}
}
