package persistence

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/ledgerwatch/erigon/cl/clparams"
	"github.com/ledgerwatch/erigon/cl/cltypes"
	"github.com/ledgerwatch/erigon/cl/phase1/execution_client"
	"github.com/ledgerwatch/erigon/cl/sentinel/peers"
	"github.com/ledgerwatch/erigon/cl/utils"
	"github.com/ledgerwatch/erigon/cmd/sentinel/sentinel/communication/ssz_snappy"
	"github.com/ledgerwatch/erigon/common/dbutils"
	"github.com/spf13/afero"
)

type beaconChainDatabaseFilesystem struct {
	fs  afero.Fs
	cfg *clparams.BeaconChainConfig

	networkEncoding bool // same encoding as reqresp

	// TODO(Giulio2002): actually make decoupling possible
	_ execution_client.ExecutionEngine
}

func NewbeaconChainDatabaseFilesystem(fs afero.Fs, cfg *clparams.BeaconChainConfig) BeaconChainDatabase {
	return beaconChainDatabaseFilesystem{
		fs:              fs,
		cfg:             cfg,
		networkEncoding: false,
	}
}

func (b beaconChainDatabaseFilesystem) GetRange(ctx context.Context, from uint64, count uint64) ([]*peers.PeeredObject[*cltypes.SignedBeaconBlock], error) {
	panic("not imlemented")
}

func (b beaconChainDatabaseFilesystem) PurgeRange(ctx context.Context, from uint64, count uint64) error {
	panic("not imlemented")
}

func (b beaconChainDatabaseFilesystem) WriteBlock(block *cltypes.SignedBeaconBlock) error {
	folderPath, path := SlotToPaths(block.Block.Slot, b.cfg)
	// ignore this error... reason: windows
	_ = b.fs.MkdirAll(folderPath, 0o755)
	fp, err := b.fs.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0o755)
	if err != nil {
		return err
	}
	defer fp.Close()
	err = fp.Truncate(0)
	if err != nil {
		return err
	}
	_, err = fp.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}
	if b.networkEncoding { // 10x bigger but less latency
		err = ssz_snappy.EncodeAndWrite(fp, block)
		if err != nil {
			return err
		}
	} else {
		if block.Version() >= clparams.BellatrixVersion {
			// Need to reference EL somehow on read.
			if _, err := fp.Write(block.Block.Body.ExecutionPayload.BlockHash[:]); err != nil {
				return err
			}
			if _, err := fp.Write(dbutils.EncodeBlockNumber(block.Block.Body.ExecutionPayload.BlockNumber)); err != nil {
				return err
			}
		}
		encoded, err := block.EncodeForStorage(nil)
		if err != nil {
			return err
		}
		if _, err := fp.Write(utils.CompressSnappy(encoded)); err != nil {
			return err
		}
	}

	err = fp.Sync()
	if err != nil {
		return err
	}
	return nil
}

// SlotToPaths define the file structure to store a block
//
// superEpoch = floor(slot / (epochSize ^ 2))
// epoch =  floot(slot / epochSize)
// file is to be stored at
// "/signedBeaconBlock/{superEpoch}/{epoch}/{slot}.ssz_snappy"
func SlotToPaths(slot uint64, config *clparams.BeaconChainConfig) (folderPath string, filePath string) {

	superEpoch := slot / (config.SlotsPerEpoch * config.SlotsPerEpoch)
	epoch := slot / config.SlotsPerEpoch

	folderPath = path.Clean(fmt.Sprintf("%d/%d", superEpoch, epoch))
	filePath = path.Clean(fmt.Sprintf("%s/%d.sz", folderPath, slot))
	return
}

func ValidateEpoch(fs afero.Fs, epoch uint64, config *clparams.BeaconChainConfig) error {
	superEpoch := epoch / (config.SlotsPerEpoch)

	// the folder path is superEpoch/epoch
	folderPath := path.Clean(fmt.Sprintf("%d/%d", superEpoch, epoch))

	fi, err := afero.ReadDir(fs, folderPath)
	if err != nil {
		return err
	}
	for _, fn := range fi {
		fn.Name()
	}
	return nil
}
