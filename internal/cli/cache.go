package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/manojpannala/torbox-trakt-wrapper/pkg/cache"
	"github.com/manojpannala/torbox-trakt-wrapper/pkg/config"
)

func cacheStore() *cache.Store {
	return cache.New(config.GetCacheDir(), 0, config.Version, logger)
}

func forget(keys ...cache.Key) {
	store := cacheStore()
	for _, k := range keys {
		store.Invalidate(k)
	}
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the local listings and watch-history cache",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete every cached listing and watch history",
	RunE: func(cmd *cobra.Command, args []string) error {
		cacheStore().Clear()
		fmt.Println("✓ Cache cleared.")
		return nil
	},
}

func init() {
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}
