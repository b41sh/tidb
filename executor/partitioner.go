// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package executor

import (
	"context"
	"github.com/pingcap/errors"
	"github.com/pingcap/tidb/util/chunk"
	"sync"
)

var (
	_ Executor = &PartitionerExec{}
)

// PartitionerExec read chunks from disk
type PartitionerExec struct {
	baseExecutor

	partitionIdx    int
	chkIdx          int
	partitionInDisk *chunk.PartitionInDisk

	prepared bool
	// partitionWaitGroup is for sync partition.
	partitionWaitGroup sync.WaitGroup
}

// Open implements the Executor Open interface.
func (e *PartitionerExec) Open(ctx context.Context) error {
	if err := e.baseExecutor.Open(ctx); err != nil {
		return err
	}

	e.prepared = false
	e.partitionWaitGroup = sync.WaitGroup{}
	return nil
}

// Close implements the Executor Close interface.
func (e *PartitionerExec) Close() error {
	err := e.partitionInDisk.Close(e.partitionIdx)
	if err != nil {
		return err
	}

	err = e.baseExecutor.Close()
	return errors.Trace(err)
}

// Next implements the Executor Next interface.
// PartitionerExec get chunk from *chunk.PartitionInDisk previously stored in disk
// on every Next call.
func (e *PartitionerExec) Next(ctx context.Context, req *chunk.Chunk) (err error) {
	if !e.prepared {
		e.partitionWaitGroup.Add(1)
		e.prepared = true
	}

	chkNum, err := e.partitionInDisk.NumChunks(e.partitionIdx)
	if err != nil {
		return errors.Trace(err)
	}
	if e.chkIdx >= chkNum {
		e.partitionWaitGroup.Done()
		e.prepared = false
		return nil
	}
	if err := e.partitionInDisk.GetChunk(e.partitionIdx, e.chkIdx, req); err != nil {
		return errors.Trace(err)
	}
	e.chkIdx++
	return nil
}
