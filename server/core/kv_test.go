// Copyright 2016 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package core

import (
	"fmt"
	"math"

	. "github.com/pingcap/check"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/pkg/errors"
)

var _ = Suite(&testKVSuite{})

type testKVSuite struct {
}

func (s *testKVSuite) TestBasic(c *C) {
	kv := NewKV(NewMemoryKV())

	c.Assert(kv.storePath(123), Equals, "raft/s/00000000000000000123")
	c.Assert(regionPath(123), Equals, "raft/r/00000000000000000123")

	meta := &metapb.Cluster{Id: 123}
	ok, err := kv.LoadMeta(meta)
	c.Assert(ok, IsFalse)
	c.Assert(err, IsNil)
	c.Assert(kv.SaveMeta(meta), IsNil)
	newMeta := &metapb.Cluster{}
	ok, err = kv.LoadMeta(newMeta)
	c.Assert(ok, IsTrue)
	c.Assert(err, IsNil)
	c.Assert(newMeta, DeepEquals, meta)

	store := &metapb.Store{Id: 123}
	ok, err = kv.LoadStore(123, store)
	c.Assert(ok, IsFalse)
	c.Assert(err, IsNil)
	c.Assert(kv.SaveStore(store), IsNil)
	newStore := &metapb.Store{}
	ok, err = kv.LoadStore(123, newStore)
	c.Assert(ok, IsTrue)
	c.Assert(err, IsNil)
	c.Assert(newStore, DeepEquals, store)

	region := &metapb.Region{Id: 123}
	ok, err = kv.LoadRegion(123, region)
	c.Assert(ok, IsFalse)
	c.Assert(err, IsNil)
	c.Assert(kv.SaveRegion(region), IsNil)
	newRegion := &metapb.Region{}
	ok, err = kv.LoadRegion(123, newRegion)
	c.Assert(ok, IsTrue)
	c.Assert(err, IsNil)
	c.Assert(newRegion, DeepEquals, region)
	err = kv.DeleteRegion(region)
	c.Assert(err, IsNil)
	ok, err = kv.LoadRegion(123, newRegion)
	c.Assert(ok, IsFalse)
	c.Assert(err, IsNil)
}

func mustSaveStores(c *C, kv *KV, n int) []*metapb.Store {
	stores := make([]*metapb.Store, 0, n)
	for i := 0; i < n; i++ {
		store := &metapb.Store{Id: uint64(i)}
		stores = append(stores, store)
	}

	for _, store := range stores {
		c.Assert(kv.SaveStore(store), IsNil)
	}

	return stores
}

func (s *testKVSuite) TestLoadStores(c *C) {
	kv := NewKV(NewMemoryKV())
	cache := NewStoresInfo()

	n := 10
	stores := mustSaveStores(c, kv, n)
	c.Assert(kv.LoadStores(cache), IsNil)

	c.Assert(cache.GetStoreCount(), Equals, n)
	for _, store := range cache.GetMetaStores() {
		c.Assert(store, DeepEquals, stores[store.GetId()])
	}
}

func (s *testKVSuite) TestStoreWeight(c *C) {
	kv := NewKV(NewMemoryKV())
	cache := NewStoresInfo()
	const n = 3

	mustSaveStores(c, kv, n)
	c.Assert(kv.SaveStoreWeight(1, 2.0, 3.0), IsNil)
	c.Assert(kv.SaveStoreWeight(2, 0.2, 0.3), IsNil)
	c.Assert(kv.LoadStores(cache), IsNil)
	leaderWeights := []float64{1.0, 2.0, 0.2}
	regionWeights := []float64{1.0, 3.0, 0.3}
	for i := 0; i < n; i++ {
		c.Assert(cache.GetStore(uint64(i)).GetLeaderWeight(), Equals, leaderWeights[i])
		c.Assert(cache.GetStore(uint64(i)).GetRegionWeight(), Equals, regionWeights[i])
	}
}

func mustSaveRegions(c *C, kv *KV, n int) []*metapb.Region {
	regions := make([]*metapb.Region, 0, n)
	for i := 0; i < n; i++ {
		region := newTestRegionMeta(uint64(i))
		regions = append(regions, region)
	}

	for _, region := range regions {
		c.Assert(kv.SaveRegion(region), IsNil)
	}

	return regions
}

func (s *testKVSuite) TestLoadRegions(c *C) {
	kv := NewKV(NewMemoryKV())
	cache := NewRegionsInfo()

	n := 10
	regions := mustSaveRegions(c, kv, n)
	c.Assert(kv.LoadRegions(cache), IsNil)

	c.Assert(cache.GetRegionCount(), Equals, n)
	for _, region := range cache.GetMetaRegions() {
		c.Assert(region, DeepEquals, regions[region.GetId()])
	}
}

func (s *testKVSuite) TestLoadRegionsExceedRangeLimit(c *C) {
	kv := NewKV(&KVWithMaxRangeLimit{KVBase: NewMemoryKV(), rangeLimit: 500})
	cache := NewRegionsInfo()

	n := 1000
	regions := mustSaveRegions(c, kv, n)
	c.Assert(kv.LoadRegions(cache), IsNil)
	c.Assert(cache.GetRegionCount(), Equals, n)
	for _, region := range cache.GetMetaRegions() {
		c.Assert(region, DeepEquals, regions[region.GetId()])
	}
}

func (s *testKVSuite) TestLoadGCSafePoint(c *C) {
	kv := NewKV(NewMemoryKV())
	testData := []uint64{0, 1, 2, 233, 2333, 23333333333, math.MaxUint64}

	r, e := kv.LoadGCSafePoint()
	c.Assert(r, Equals, uint64(0))
	c.Assert(e, IsNil)
	for _, safePoint := range testData {
		err := kv.SaveGCSafePoint(safePoint)
		c.Assert(err, IsNil)
		safePoint1, err := kv.LoadGCSafePoint()
		c.Assert(err, IsNil)
		c.Assert(safePoint, Equals, safePoint1)
	}
}

type KVWithMaxRangeLimit struct {
	KVBase
	rangeLimit int
}

func (kv *KVWithMaxRangeLimit) LoadRange(key, endKey string, limit int) ([]string, error) {
	if limit > kv.rangeLimit {
		return nil, errors.Errorf("limit %v exceed max rangeLimit %v", limit, kv.rangeLimit)
	}
	return kv.KVBase.LoadRange(key, endKey, limit)
}

func newTestRegionMeta(regionID uint64) *metapb.Region {
	return &metapb.Region{
		Id:       regionID,
		StartKey: []byte(fmt.Sprintf("%20d", regionID)),
		EndKey:   []byte(fmt.Sprintf("%20d", regionID+1)),
	}
}
