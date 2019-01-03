package brickmanager

import (
	"context"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/pfsprovider/ansible"
	"github.com/RSE-Cambridge/data-acc/internal/pkg/registry"
	"log"
	"strings"
)

func setupBrickEventHandlers(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry,
	hostname string) {

	newBricks := poolRegistry.GetNewHostBrickAllocations(context.Background(), hostname)
	go func() {
		for brick := range newBricks {
			if brick.AllocatedIndex == 0 {
				go processNewPrimaryBlock(poolRegistry, volumeRegistry, &brick)
			}
		}
		log.Panic("we appear to have stopped watching for new bricks")
	}()

	allocations, err := poolRegistry.GetAllocationsForHost(hostname)
	if err != nil {
		if !strings.Contains(err.Error(), "unable to find any values") {
			log.Panic(err)
		}
	}

	for _, allocation := range allocations {
		if allocation.AllocatedIndex == 0 {
			volume, err := volumeRegistry.Volume(allocation.AllocatedVolume)
			if err != nil {
				log.Panicf("unable to find volume for allocation %+v", allocation)
			}
			log.Println("We host a primary brick for:", volume.Name, volume)
			if volume.State == registry.BricksProvisioned || volume.State == registry.DataInComplete {
				log.Println("Start watch for changes to volume again:", volume.Name)
				watchForVolumeChanges(poolRegistry, volumeRegistry, volume)
			}
			if volume.State == registry.DeleteRequested {
				log.Println("Complete pending delete request for volume:", volume.Name)
				processDelete(poolRegistry, volumeRegistry, volume)
			}
		}
	}

	// TODO what about catching up with changes while we were down, make sure system in correct state!!
}

func processNewPrimaryBlock(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry,
	new *registry.BrickAllocation) {
	volume, err := volumeRegistry.Volume(new.AllocatedVolume)
	if err != nil {
		log.Printf("Could not file volume: %s because: %s\n", new.AllocatedVolume, err)
		return
	}
	log.Println("Found new volume to watch:", volume.Name)
	log.Println(volume)

	watchForVolumeChanges(poolRegistry, volumeRegistry, volume)

	// Move to new state, ignored by above watch
	provisionNewVolume(poolRegistry, volumeRegistry, volume)
}

func watchForVolumeChanges(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry,
	volume registry.Volume) {

	// TODO: watch from version associated with above volume to avoid any missed events
	volumeRegistry.WatchVolumeChanges(string(volume.Name), func(old *registry.Volume, new *registry.Volume) bool {
		if old != nil && new != nil {
			if new.State != old.State {
				switch new.State {
				case registry.DataInRequested:
					processDataIn(volumeRegistry, *new)
				case registry.DataOutRequested:
					processDataOut(volumeRegistry, *new)
				case registry.DeleteRequested:
					processDelete(poolRegistry, volumeRegistry, *new)
				case registry.BricksDeleted:
					log.Println("Volume", new.Name, "deleted, stop listening for events now.")
					return true
				default:
					// Ignore the state changes we triggered
					log.Println(". ingore volume:", volume.Name, "state move:", old.State, "->", new.State)
				}
			}

			if len(new.Attachments) > len(old.Attachments) {
				var attachRequested []registry.Attachment
				for _, newAttachment := range new.Attachments {
					isNew := false
					if old.Attachments == nil {
						isNew = true
					} else {
						_, ok := old.FindMatchingAttachment(newAttachment)
						isNew = !ok
					}
					if isNew && newAttachment.State == registry.RequestAttach {
						attachRequested = append(attachRequested, newAttachment)
					}
				}
				if attachRequested != nil && len(attachRequested) > 0 {
					processAttach(poolRegistry, volumeRegistry, *new, attachRequested)
				}
			}

			if len(new.Attachments) == len(old.Attachments) && new.Attachments != nil && old.Attachments != nil {
				var detachRequested []registry.Attachment
				for _, newAttachment := range new.Attachments {
					oldAttachment, ok := old.FindMatchingAttachment(newAttachment)
					if ok && newAttachment.State == registry.RequestDetach && oldAttachment.State == registry.Attached {
						detachRequested = append(detachRequested, newAttachment)
					}
				}
				if len(detachRequested) > 0 {
					processDetach(poolRegistry, volumeRegistry, *new, detachRequested)
				}
			}
		}
		// keep watching
		return false
	})
}

func handleError(volumeRegistry registry.VolumeRegistry, volume registry.Volume, err error) {
	if err != nil {
		log.Println("Error provisioning", volume.Name, err)
		err = volumeRegistry.UpdateState(volume.Name, registry.Error) // TODO record an error string?
		if err != nil {
			log.Println("Unable to move volume", volume.Name, "to Error state")
		}
	}
}

// TODO: should not be hardcoded here
var FSType = ansible.Lustre
var plugin = ansible.GetPlugin(FSType)

func provisionNewVolume(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry, volume registry.Volume) {
	if volume.State != registry.Registered {
		log.Println("Volume in bad initial state:", volume.Name)
		return
	}

	bricks, err := poolRegistry.GetAllocationsForVolume(volume.Name)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	// Before we provision the bricks, notify that we have seen the volume
	err = volumeRegistry.UpdateState(volume.Name, registry.BricksAllocated)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	err = plugin.VolumeProvider().SetupVolume(volume, bricks)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	err = volumeRegistry.UpdateState(volume.Name, registry.BricksProvisioned)
	handleError(volumeRegistry, volume, err)
}

func processDataIn(volumeRegistry registry.VolumeRegistry, volume registry.Volume) {
	err := plugin.VolumeProvider().CopyDataIn(volume)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	err = volumeRegistry.UpdateState(volume.Name, registry.DataInComplete)
	handleError(volumeRegistry, volume, err)
}

func processAttach(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry, volume registry.Volume,
	attachments []registry.Attachment) {

	bricks, err := poolRegistry.GetAllocationsForVolume(volume.Name)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}
	err = plugin.Mounter().Mount(volume, bricks) // TODO pass down specific attachments?
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	// TODO: this is really odd, mount should probably do this?
	updates := []registry.Attachment{}
	for _, attachment := range attachments {
		if attachment.State == registry.RequestAttach {
			attachment.State = registry.Attached
			updates = append(updates, attachment)
		}

	}
	// TODO: what can we do if we hit an error here?
	volumeRegistry.UpdateVolumeAttachments(volume.Name, updates)
}

func processDetach(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry, volume registry.Volume,
	attachments []registry.Attachment) {

	bricks, err := poolRegistry.GetAllocationsForVolume(volume.Name)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	err = plugin.Mounter().Unmount(volume, bricks) // TODO pass down specific attachments?
	if err != nil {
		// TODO: update specific attachment into an error state?
		handleError(volumeRegistry, volume, err)
	}

	var updates []registry.Attachment
	for _, attachment := range attachments {
		if attachment.State == registry.RequestDetach {
			attachment.State = registry.Detached
			updates = append(updates, attachment)
		}
	}
	volumeRegistry.UpdateVolumeAttachments(volume.Name, updates)
}

func processDataOut(volumeRegistry registry.VolumeRegistry, volume registry.Volume) {
	err := plugin.VolumeProvider().CopyDataOut(volume)
	if err != nil {
		handleError(volumeRegistry, volume, err)
	}

	err = volumeRegistry.UpdateState(volume.Name, registry.DataOutComplete)
	handleError(volumeRegistry, volume, err)
}

func processDelete(poolRegistry registry.PoolRegistry, volumeRegistry registry.VolumeRegistry, volume registry.Volume) {
	bricks, err := poolRegistry.GetAllocationsForVolume(volume.Name)
	if err != nil {
		handleError(volumeRegistry, volume, err)
		return
	}

	err = plugin.VolumeProvider().TeardownVolume(volume, bricks)
	if err != nil {
		handleError(volumeRegistry, volume, err)
	}

	err = volumeRegistry.UpdateState(volume.Name, registry.BricksDeleted)
	handleError(volumeRegistry, volume, err)
}
