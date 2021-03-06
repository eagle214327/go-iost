package txpool

import (
	"testing"
	"time"

	"os"

	. "github.com/golang/mock/gomock"
	"github.com/iost-official/go-iost/account"
	"github.com/iost-official/go-iost/common"
	"github.com/iost-official/go-iost/core/block"
	"github.com/iost-official/go-iost/core/blockcache"
	"github.com/iost-official/go-iost/core/global"
	"github.com/iost-official/go-iost/core/mocks"
	"github.com/iost-official/go-iost/core/tx"
	"github.com/iost-official/go-iost/crypto"
	"github.com/iost-official/go-iost/db/mocks"
	"github.com/iost-official/go-iost/ilog"
	"github.com/iost-official/go-iost/p2p"
	"github.com/iost-official/go-iost/p2p/mocks"
	"github.com/iost-official/go-iost/vm/database"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	dbPath1 = "TXDB"
	dbPath2 = "StateDB"
	dbPath3 = "BlockChainDB"
	walPath = "BlockCacheWAl"
)

func (pool *TxPImpl) testPendingTxsNum() int64 {
	return int64(pool.pendingTx.Size())
}

func (pool *TxPImpl) testBlockListNum() int64 {
	var r int64
	pool.blockList.Range(func(key, value interface{}) bool {
		r++
		return true
	})
	return r
}

func TestNewTxPImpl(t *testing.T) {
	Convey("test NewTxPoolServer", t, func() {
		ctl := NewController(t)
		p2pMock := p2p_mock.NewMockService(ctl)

		p2pCh := make(chan p2p.IncomingMessage, 100)
		p2pMock.EXPECT().Broadcast(Any(), Any(), Any()).AnyTimes()
		p2pMock.EXPECT().Register(Any(), Any()).Return(p2pCh)

		var accountList []*account.KeyPair
		var witnessList []string
		var witnessInfo []string
		acc := common.Base58Decode("3BZ3HWs2nWucCCvLp7FRFv1K7RR3fAjjEQccf9EJrTv4")
		newAccount, err := account.NewKeyPair(acc, crypto.Secp256k1)
		if err != nil {
			panic("account.NewKeyPair error")
		}
		accountList = append(accountList, newAccount)
		witnessInfo = append(witnessInfo, newAccount.ReadablePubkey())
		witnessInfo = append(witnessInfo, "100000")
		witnessList = append(witnessList, newAccount.ReadablePubkey())
		for i := 1; i < 3; i++ {
			newAccount, err := account.NewKeyPair(nil, crypto.Secp256k1)
			if err != nil {
				panic("account.NewKeyPair error")
			}
			accountList = append(accountList, newAccount)
			witnessList = append(witnessList, newAccount.ReadablePubkey())
			witnessInfo = append(witnessInfo, newAccount.ReadablePubkey())
			witnessInfo = append(witnessInfo, "100000")
		}
		//conf := &common.Config{
		//	DB:      &common.DBConfig{},
		//	Genesis: &common.GenesisConfig{CreateGenesis: true, WitnessInfo: witnessInfo},
		//}
		//gl, err := gbl.New(conf)

		statedb := db_mock.NewMockMVCCDB(ctl)
		statedb.EXPECT().Flush(Any()).AnyTimes().Return(nil)
		statedb.EXPECT().Fork().AnyTimes().Return(statedb)
		statedb.EXPECT().Checkout(Any()).AnyTimes().Return(true)
		statedb.EXPECT().Close().AnyTimes()

		statedb.EXPECT().Get("state", "b-vote_producer.iost-"+"pendingBlockNumber").AnyTimes().DoAndReturn(func(table string, key string) (string, error) {
			return database.MustMarshal("4"), nil
		})
		statedb.EXPECT().Get("state", "b-vote_producer.iost-"+"pendingProducerList").AnyTimes().DoAndReturn(func(table string, key string) (string, error) {
			return database.MustMarshal("[\"a1\",\"a2\",\"a3\",\"a4\"]"), nil
		})
		statedb.EXPECT().Get("state", Any()).AnyTimes().DoAndReturn(func(table string, key string) (string, error) {
			return database.MustMarshal(`{"loc":"11","url":"22","netId":"33","online":true,"score":0,"votes":0}`), nil
		})

		b := genBlocks(accountList, witnessList, 1, 11, true)
		base := core_mock.NewMockChain(ctl)
		base.EXPECT().Top().AnyTimes().Return(b[0], nil)
		base.EXPECT().Push(Any()).AnyTimes().Return(nil)
		base.EXPECT().Length().AnyTimes().Return(int64(1))
		base.EXPECT().Close().AnyTimes()
		base.EXPECT().AllDelaytx().AnyTimes().Return(nil, nil)

		gbl := core_mock.NewMockBaseVariable(ctl)
		gbl.EXPECT().StateDB().AnyTimes().Return(statedb)
		gbl.EXPECT().BlockChain().AnyTimes().Return(base)
		gbl.EXPECT().Mode().AnyTimes().Return(global.ModeNormal)
		config := common.Config{
			DB: &common.DBConfig{
				LdbPath: "./",
			},
		}
		gbl.EXPECT().Config().AnyTimes().Return(&config)

		So(err, ShouldBeNil)
		blockcache.CleanBlockCacheWAL()
		BlockCache, err := blockcache.NewBlockCache(gbl)
		So(err, ShouldBeNil)

		txPool, err := NewTxPoolImpl(gbl, BlockCache, p2pMock)
		So(err, ShouldBeNil)

		txPool.Start()
		Convey("AddTx", func() {

			t := genTx(accountList[0], tx.MaxExpiration)
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)
			err := txPool.AddTx(t)
			So(err, ShouldBeNil)
			So(txPool.testPendingTxsNum(), ShouldEqual, 1)
			err = txPool.AddTx(t)
			So(err, ShouldEqual, ErrDupPendingTx)
		})
		Convey("txTimeOut", func() {

			t := genTx(accountList[0], tx.MaxExpiration)

			b := t.IsExpired(time.Now().UnixNano())
			So(b, ShouldBeFalse)

			t.Time -= int64(tx.MaxExpiration)
			b = t.IsExpired(time.Now().UnixNano())
			So(b, ShouldBeTrue)

			t = genTx(accountList[0], tx.MaxExpiration)

			t.Expiration -= int64(tx.MaxExpiration * 3)
			b = t.IsExpired(time.Now().UnixNano())
			So(b, ShouldBeTrue)
		})
		Convey("delTimeOutTx", func() {

			t := genTx(accountList[0], int64(30*time.Millisecond))
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)

			err := txPool.AddTx(t)
			So(err, ShouldBeNil)
			So(txPool.testPendingTxsNum(), ShouldEqual, 1)
			time.Sleep(50 * time.Millisecond)
			txPool.clearTimeoutTx()
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)
		})
		Convey("ExistTxs FoundPending", func() {

			t := genTx(accountList[0], tx.MaxExpiration)
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)
			err := txPool.AddTx(t)
			So(err, ShouldBeNil)
			So(txPool.testPendingTxsNum(), ShouldEqual, 1)
			r1 := txPool.ExistTxs(t.Hash(), nil)
			So(r1, ShouldEqual, FoundPending)
		})
		Convey("ExistTxs FoundChain", func() {

			txCnt := 10
			b := genBlocks(accountList, witnessList, 1, txCnt, true)
			//ilog.Debug(("FoundChain", b[0].HeadHash())

			bcn := blockcache.NewBCN(nil, b[0])
			So(txPool.testBlockListNum(), ShouldEqual, 0)

			err := txPool.AddLinkedNode(bcn)
			So(err, ShouldBeNil)

			// need delay
			for i := 0; i < 20; i++ {
				time.Sleep(20 * time.Millisecond)
				if txPool.testBlockListNum() == 1 {
					break
				}
			}

			So(txPool.testBlockListNum(), ShouldEqual, 1)
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)
			for i := 0; i < txCnt; i++ {
				r1 := txPool.ExistTxs(b[0].Txs[i].Hash(), bcn.Block)
				So(r1, ShouldEqual, FoundChain)
			}

			t := genTx(accountList[0], tx.MaxExpiration)
			r1 := txPool.ExistTxs(t.Hash(), bcn.Block)
			So(r1, ShouldEqual, NotFound)
		})
		stopTest(gbl)
	})

}

func TestNewTxPImplB(t *testing.T) {
	Convey("test NewTxPoolServer", t, func() {
		ctl := NewController(t)
		p2pMock := p2p_mock.NewMockService(ctl)

		p2pCh := make(chan p2p.IncomingMessage, 100)
		p2pMock.EXPECT().Broadcast(Any(), Any(), Any()).AnyTimes()
		p2pMock.EXPECT().Register(Any(), Any()).Return(p2pCh)

		var accountList []*account.KeyPair
		var witnessList []string
		var witnessInfo []string
		acc := common.Base58Decode("3BZ3HWs2nWucCCvLp7FRFv1K7RR3fAjjEQccf9EJrTv4")
		newAccount, err := account.NewKeyPair(acc, crypto.Secp256k1)
		if err != nil {
			panic("account.NewKeyPair error")
		}
		accountList = append(accountList, newAccount)
		witnessInfo = append(witnessInfo, newAccount.ReadablePubkey())
		witnessInfo = append(witnessInfo, "100000")
		witnessList = append(witnessList, newAccount.ReadablePubkey())
		for i := 1; i < 3; i++ {
			newAccount, err := account.NewKeyPair(nil, crypto.Secp256k1)
			if err != nil {
				panic("account.NewKeyPair error")
			}
			accountList = append(accountList, newAccount)
			witnessList = append(witnessList, newAccount.ReadablePubkey())
			witnessInfo = append(witnessInfo, newAccount.ReadablePubkey())
			witnessInfo = append(witnessInfo, "100000")
		}
		//conf := &common.Config{
		//	DB:      &common.DBConfig{},
		//	Genesis: &common.GenesisConfig{CreateGenesis: true, WitnessInfo: witnessInfo},
		//}
		//gl, err := gbl.New(conf)

		statedb := db_mock.NewMockMVCCDB(ctl)
		statedb.EXPECT().Flush(Any()).AnyTimes().Return(nil)
		statedb.EXPECT().Fork().AnyTimes().Return(statedb)
		statedb.EXPECT().Checkout(Any()).AnyTimes().Return(true)
		statedb.EXPECT().Close().AnyTimes()

		statedb.EXPECT().Get("state", "b-vote_producer.iost-"+"pendingBlockNumber").AnyTimes().DoAndReturn(func(table string, key string) (string, error) {
			return database.MustMarshal("4"), nil
		})
		statedb.EXPECT().Get("state", "b-vote_producer.iost-"+"pendingProducerList").AnyTimes().DoAndReturn(func(table string, key string) (string, error) {
			return database.MustMarshal("[\"a1\",\"a2\",\"a3\",\"a4\"]"), nil
		})
		statedb.EXPECT().Get("state", Any()).AnyTimes().DoAndReturn(func(table string, key string) (string, error) {
			return database.MustMarshal(`{"loc":"11","url":"22","netId":"33","online":true,"score":0,"votes":0}`), nil
		})

		b := genBlocks(accountList, witnessList, 1, 11, true)
		base := core_mock.NewMockChain(ctl)
		base.EXPECT().Top().AnyTimes().Return(b[0], nil)
		base.EXPECT().Push(Any()).AnyTimes().Return(nil)
		base.EXPECT().Length().AnyTimes().Return(int64(1))
		base.EXPECT().Close().AnyTimes()
		base.EXPECT().AllDelaytx().AnyTimes().Return(nil, nil)

		gbl := core_mock.NewMockBaseVariable(ctl)
		gbl.EXPECT().StateDB().AnyTimes().Return(statedb)
		gbl.EXPECT().BlockChain().AnyTimes().Return(base)
		gbl.EXPECT().Mode().AnyTimes().Return(global.ModeNormal)
		config := common.Config{
			DB: &common.DBConfig{
				LdbPath: "./",
			},
		}
		gbl.EXPECT().Config().AnyTimes().Return(&config)

		So(err, ShouldBeNil)
		blockcache.CleanBlockCacheWAL()
		BlockCache, err := blockcache.NewBlockCache(gbl)
		So(err, ShouldBeNil)

		txPool, err := NewTxPoolImpl(gbl, BlockCache, p2pMock)
		So(err, ShouldBeNil)

		txPool.Start()
		Convey("delPending", func() {

			t := genTx(accountList[0], tx.MaxExpiration)
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)
			err := txPool.AddTx(t)
			So(err, ShouldBeNil)
			So(txPool.testPendingTxsNum(), ShouldEqual, 1)
			e := txPool.DelTx(t.Hash())
			So(e, ShouldBeNil)
			So(txPool.testPendingTxsNum(), ShouldEqual, 0)

		})

		Convey("doChainChange", func() {

			txCnt := 10
			blockCnt := 3
			blockList := genBlocks(accountList, witnessList, blockCnt, txCnt, true)
			txPool.blockCache.Head().Head.Number = 0
			for i := 0; i < blockCnt; i++ {
				//ilog.Info(("hash:", blockList[i].HeadHash(), " parentHash:", blockList[i].Head.ParentHash)
				bcn := BlockCache.Add(blockList[i])
				So(bcn, ShouldNotBeNil)

				err = txPool.AddLinkedNode(bcn)
				So(err, ShouldBeNil)
			}
			forkBlock := genSingleBlock(accountList, witnessList, blockList[1].HeadHash(), 6)
			//ilog.Debug(("Sing hash:", forkBlock.HeadHash(), " Sing parentHash:", forkBlock.Head.ParentHash)
			bcn := BlockCache.Add(forkBlock)
			So(bcn, ShouldNotBeNil)

			for i := 0; i < 6-3; i++ {
				err := txPool.AddTx(forkBlock.Txs[i])
				So(err, ShouldBeNil)
			}

			So(txPool.testPendingTxsNum(), ShouldEqual, 3)
			// fork chain
			err = txPool.AddLinkedNode(bcn)
			So(err, ShouldBeNil)
			// need delay
			for i := 0; i < 20; i++ {
				time.Sleep(20 * time.Millisecond)
				if txPool.testBlockListNum() == 10 {
					break
				}
			}

			So(txPool.testPendingTxsNum(), ShouldEqual, 10)
		})

		Convey("rbtree", func() {
			t1 := genTx(newAccount, tx.MaxExpiration)
			t2 := genTx(newAccount, tx.MaxExpiration)
			t3 := genTx(newAccount, tx.MaxExpiration)
			t4 := genTx(newAccount, tx.MaxExpiration)
			t5 := genTx(newAccount, tx.MaxExpiration)
			t1.GasRatio = 100
			t2.GasRatio = 200
			t3.GasRatio = 200
			t3.Time = t2.Time + 1
			t4.GasRatio = 400
			t5.GasRatio = 500

			sig1, err := tx.SignTxContent(t1, newAccount.ReadablePubkey(), newAccount)
			So(err, ShouldBeNil)
			t1.Signs = []*crypto.Signature{sig1}
			t1, err = tx.SignTx(t1, newAccount.ReadablePubkey(), []*account.KeyPair{newAccount})
			So(err, ShouldBeNil)

			sig2, err := tx.SignTxContent(t2, newAccount.ReadablePubkey(), newAccount)
			So(err, ShouldBeNil)
			t2.Signs = []*crypto.Signature{sig2}
			t2, err = tx.SignTx(t2, newAccount.ReadablePubkey(), []*account.KeyPair{newAccount})
			So(err, ShouldBeNil)

			sig3, err := tx.SignTxContent(t3, newAccount.ReadablePubkey(), newAccount)
			So(err, ShouldBeNil)
			t3.Signs = []*crypto.Signature{sig3}
			t3, err = tx.SignTx(t3, newAccount.ReadablePubkey(), []*account.KeyPair{newAccount})
			So(err, ShouldBeNil)

			sig4, err := tx.SignTxContent(t4, newAccount.ReadablePubkey(), newAccount)
			So(err, ShouldBeNil)
			t4.Signs = []*crypto.Signature{sig4}
			t4, err = tx.SignTx(t4, newAccount.ReadablePubkey(), []*account.KeyPair{newAccount})
			So(err, ShouldBeNil)

			sig5, err := tx.SignTxContent(t5, newAccount.ReadablePubkey(), newAccount)
			So(err, ShouldBeNil)
			t5.Signs = []*crypto.Signature{sig5}
			t5, err = tx.SignTx(t5, newAccount.ReadablePubkey(), []*account.KeyPair{newAccount})
			So(err, ShouldBeNil)

			println("before add tx")
			err = txPool.AddTx(t4)
			So(err, ShouldBeNil)
			err = txPool.AddTx(t2)
			So(err, ShouldBeNil)
			err = txPool.AddTx(t5)
			So(err, ShouldBeNil)
			err = txPool.AddTx(t1)
			So(err, ShouldBeNil)
			err = txPool.AddTx(t3)
			So(err, ShouldBeNil)

			pt, _ := txPool.PendingTx()
			iter := pt.Iter()
			trx, ok := iter.Next()
			for _, expectTx := range []*tx.Tx{t5, t4, t2, t3, t1} {
				So(ok, ShouldBeTrue)
				So(common.Base58Encode(expectTx.Hash()), ShouldEqual, common.Base58Encode(trx.Hash()))
				trx, ok = iter.Next()
			}
			So(ok, ShouldBeFalse)

		})

		//
		//Convey("concurrent", func() {
		//	txCnt := 10
		//	blockCnt := 100
		//	bl := genNodes(accountList, witnessList, blockCnt, txCnt, true)
		//	ch := make(chan int, 4)
		//	//fmt.Println("genNodes impl")
		//	go func() {
		//		for _, bcn := range bl {
		//			txPool.AddLinkedNode(bcn, bcn)
		//		}
		//		ch <- 1
		//	}()
		//
		//	go func() {
		//		for i := 0; i < 100; i++ {
		//			t := genTx(accountList[0], Expiration)
		//			txPool.AddTx(t)
		//		}
		//		ch <- 2
		//	}()
		//
		//	go func() {
		//		for i := 0; i < 10000; i++ {
		//			txPool.PendingTxs(10000000)
		//		}
		//		ch <- 3
		//	}()
		//	////time.Sleep(5*time.Second)
		//
		//	t := genTx(accountList[0], Expiration)
		//	txPool.AddTx(t)
		//	go func() {
		//		for i := 0; i < 10000; i++ {
		//			txPool.ExistTxs(t.Hash(), bl[blockCnt-10].Block)
		//		}
		//		ch <- 4
		//	}()
		//
		//	for i := 0; i < 4; i++ {
		//		<-ch
		//		//fmt.Println("ch :", a)
		//	}
		//
		//})

		stopTest(gbl)
	})

}

//result 55.3 ns/op
func BenchmarkAddBlock(b *testing.B) {
	_, accountList, witnessList, txPool, gl := envInit(b)
	listTxCnt := 500
	blockList := genBlocks(accountList, witnessList, 1, listTxCnt, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		txPool.addBlock(blockList[0])
	}

	b.StopTimer()
	stopTest(gl)

}

//result 472185 ns/op  tps:2147
// no verify 17730 ns/op tps:58823
func BenchmarkAddTx(b *testing.B) {
	_, accountList, witnessList, txPool, gl := envInit(b)
	listTxCnt := 10
	blockCnt := 100
	blockList := genNodes(accountList, witnessList, blockCnt, listTxCnt, true)

	for i := 0; i < blockCnt; i++ {
		txPool.AddLinkedNode(blockList[i])
	}
	time.Sleep(200 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		t := genTx(accountList[0], tx.MaxExpiration)
		b.StartTimer()

		txPool.verifyDuplicate(t)
		txPool.pendingTx.Add(t)
	}

	b.StopTimer()
	stopTest(gl)
}

//result 4445 ns/op
func BenchmarkDecodeTx(b *testing.B) {
	acc := common.Base58Decode("3BZ3HWs2nWucCCvLp7FRFv1K7RR3fAjjEQccf9EJrTv4")
	newAccount, err := account.NewKeyPair(acc, crypto.Secp256k1)
	if err != nil {
		panic("account.NewKeyPair error")
	}

	tm := genTxMsg(newAccount, tx.MaxExpiration)
	var t tx.Tx
	err = t.Decode(tm.Data())
	if err != nil {
		panic("Decode error")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {

		var t tx.Tx
		t.Decode(tm.Data())
	}

	b.StopTimer()
}

//result 3416 ns/op
func BenchmarkEncodeTx(b *testing.B) {
	acc := common.Base58Decode("3BZ3HWs2nWucCCvLp7FRFv1K7RR3fAjjEQccf9EJrTv4")
	newAccount, err := account.NewKeyPair(acc, crypto.Secp256k1)
	if err != nil {
		panic("account.NewKeyPair error")
	}

	tm := genTx(newAccount, tx.MaxExpiration)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tm.Encode()
	}

	b.StopTimer()
}

//result 3.8S ~ 4.2S  10000 tx verify
func BenchmarkVerifyTx(b *testing.B) {

	_, accountList, _, txPool, gl := envInit(b)

	t := genTx(accountList[0], tx.MaxExpiration)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < 10000; j++ {
			txPool.verifyTx(t)
		}
	}

	b.StopTimer()
	stopTest(gl)
}

//result 1 goroutine 3.8S ~ 4.2S  10000 tx verify
//result 2 goroutine 2.0S ~ 2.1S  10000 tx verify
//result 3 goroutine 1.3S ~ 1.7S  10000 tx verify
//result 5 goroutine 1.0S ~ 1.2S  10000 tx verify
//result 8 goroutine 1.0S ~ 1.3S  10000 tx verify
/*func BenchmarkConcurrentVerifyTx(b *testing.B) {
	_, accountList, _, txPool, gl := envInit(b)

	txCnt := 10000
	goCnt := 4

	t := genTxMsg(accountList[0], Expiration)

	tc := make(chan p2p.IncomingMessage, txCnt)
	rc := make(chan *tx.Tx, txCnt)

	for j := 0; j < txCnt; j++ {
		tc <- *t
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for z := 0; z < goCnt; z++ {
			b.StopTimer()
			go txPool.verifyWorkers()
			b.StartTimer()
		}

		for j := 0; j < txCnt; j++ {
			<-rc
		}

	}

	b.StopTimer()
	stopTest(gl)
}*/

func envInit(b *testing.B) (blockcache.BlockCache, []*account.KeyPair, []string, *TxPImpl, global.BaseVariable) {
	//ctl := gomock.NewController(t)

	var accountList []*account.KeyPair
	var witnessList []string

	acc := common.Base58Decode("3BZ3HWs2nWucCCvLp7FRFv1K7RR3fAjjEQccf9EJrTv4")
	newAccount, err := account.NewKeyPair(acc, crypto.Secp256k1)
	if err != nil {
		panic("account.NewKeyPair error")
	}
	accountList = append(accountList, newAccount)
	witnessList = append(witnessList, newAccount.ReadablePubkey())
	//_accId := newAccount.ID

	for i := 1; i < 3; i++ {
		newAccount, err := account.NewKeyPair(nil, crypto.Secp256k1)
		if err != nil {
			panic("account.NewKeyPair error")
		}
		accountList = append(accountList, newAccount)
		witnessList = append(witnessList, newAccount.ReadablePubkey())
	}

	config := &common.P2PConfig{
		ListenAddr: "0.0.0.0:8088",
	}

	node, _ := p2p.NewNetService(config)

	conf := &common.Config{}

	gl, _ := global.New(conf)

	blockList := genBlocks(accountList, witnessList, 1, 1, true)

	gl.BlockChain().Push(blockList[0])
	//base := core_mock.NewMockChain(ctl)
	//base.EXPECT().Top().AnyTimes().Return(blockList[0], nil)
	//base.EXPECT().Push(gomock.Any()).AnyTimes().Return(nil)

	blockcache.CleanBlockCacheWAL()
	BlockCache, _ := blockcache.NewBlockCache(gl)

	txPool, _ := NewTxPoolImpl(gl, BlockCache, node)

	txPool.Start()
	b.ResetTimer()

	return BlockCache, accountList, witnessList, txPool, gl
}

func stopTest(gl global.BaseVariable) {
	gl.StateDB().Close()
	gl.BlockChain().Close()
	os.RemoveAll(dbPath1)
	os.RemoveAll(dbPath2)
	os.RemoveAll(dbPath3)
	os.RemoveAll(walPath)
}

func genTx(a *account.KeyPair, expirationIter int64) *tx.Tx {
	actions := make([]*tx.Action, 0)
	actions = append(actions, &tx.Action{
		Contract:   "contract1",
		ActionName: "actionname1",
		Data:       "{\"num\": 1, \"message\": \"contract1\"}",
	})
	actions = append(actions, &tx.Action{
		Contract:   "contract2",
		ActionName: "actionname2",
		Data:       "1",
	})

	ex := time.Now().UnixNano() + expirationIter

	t := tx.NewTx(actions, []string{a.ReadablePubkey()}, 1000000, 100, ex, 0, 0)

	sig1, err := tx.SignTxContent(t, a.ReadablePubkey(), a)
	if err != nil {
		ilog.Debug("failed to SignTxContent")
	}

	t.Signs = append(t.Signs, sig1)

	t1, err := tx.SignTx(t, a.ReadablePubkey(), []*account.KeyPair{a})
	if err != nil {
		ilog.Debug("failed to SignTx")
	}

	if err := t1.VerifySelf(); err != nil {
		ilog.Debug("failed to t.VerifySelf(), err", err)
	}

	return t1
}

func genTxMsg(a *account.KeyPair, expirationIter int64) *p2p.IncomingMessage {
	t := genTx(a, expirationIter)

	broadTx := p2p.NewIncomingMessage("test", t.Encode(), p2p.PublishTx)

	return broadTx
}

func genBlocks(accountList []*account.KeyPair, witnessList []string, blockCnt int, txCnt int, continuity bool) (blockPool []*block.Block) {

	var hash []byte

	for i := 0; i < blockCnt; i++ {

		if continuity == false {
			hash[i%len(hash)] = byte(i % 256)
		}
		blk := block.Block{
			Txs: []*tx.Tx{},
			Head: &block.BlockHead{
				Version:             0,
				ParentHash:          hash,
				TxReceiptMerkleHash: make([]byte, 0),
				Info:                []byte(""),
				Number:              int64(i + 1),
				Witness:             witnessList[0],
				Time:                time.Now().UnixNano(),
			},
			Sign: &crypto.Signature{},
		}

		for i := 0; i < txCnt; i++ {
			blk.Txs = append(blk.Txs, genTx(accountList[0], tx.MaxExpiration))
		}

		blk.Head.TxMerkleHash = blk.CalculateTxMerkleHash()
		blk.CalculateHeadHash()

		hash = blk.HeadHash()
		blockPool = append(blockPool, &blk)
	}

	return
}

func genNodes(accountList []*account.KeyPair, witnessList []string, blockCnt int, txCnt int, continuity bool) []*blockcache.BlockCacheNode {

	var bcnList []*blockcache.BlockCacheNode

	blockList := genBlocks(accountList, witnessList, blockCnt, txCnt, continuity)

	for i := 0; i < blockCnt; i++ {
		bcn := blockcache.NewBCN(nil, blockList[i])

		bcnList = append(bcnList, bcn)
	}

	return bcnList
}

func genSingleBlock(accountList []*account.KeyPair, witnessList []string, ParentHash []byte, txCnt int) *block.Block {
	blk := block.Block{Txs: []*tx.Tx{}, Head: &block.BlockHead{
		Version:             0,
		ParentHash:          ParentHash,
		TxReceiptMerkleHash: make([]byte, 0),
		Info:                []byte(""),
		Number:              int64(1),
		Witness:             witnessList[0],
		Time:                time.Now().UnixNano(),
	}}

	for i := 0; i < txCnt; i++ {
		blk.Txs = append(blk.Txs, genTx(accountList[0], tx.MaxExpiration))
	}

	blk.Head.TxMerkleHash = blk.CalculateTxMerkleHash()
	blk.CalculateHeadHash()

	return &blk
}
