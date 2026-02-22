package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/bits"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/bits-and-blooms/bloom/v3"
	"github.com/go-faster/errors"
	pgzip "github.com/klauspost/pgzip"
	"github.com/shopspring/decimal"
	"golang.org/x/sync/errgroup"

	"github.com/xenking/oolio-kart-challenge/gen/sqlc"
	"github.com/xenking/oolio-kart-challenge/internal/storage/postgres"
)

const (
	bloomCapacity = 120_000_000
	bloomFPR      = 0.001
	numFiles      = 3
	progressEvery = 10_000_000
	minCodeLen    = 8
	maxCodeLen    = 10
)

// codeRule describes the discount rule to apply for a known coupon code.
type codeRule struct {
	discountType string
	value        string
	minItems     int32
	description  string
}

var codeRules = map[string]codeRule{
	"BIRTHDAY": {discountType: "free_lowest", value: "0", minItems: 0, description: "Birthday: free lowest item"},
	"BUYGETON": {discountType: "free_lowest", value: "0", minItems: 2, description: "Lowest item free (buy 2+)"},
	"FIFTYOFF": {discountType: "percentage", value: "50", minItems: 0, description: "50% off entire order"},
	"SIXTYOFF": {discountType: "percentage", value: "60", minItems: 0, description: "60% off entire order"},
	"FREEZAAA": {discountType: "percentage", value: "100", minItems: 0, description: "Everything free!"},
	"GNULINUX": {discountType: "percentage", value: "15", minItems: 0, description: "Open source discount: 15% off"},
	"OVER9000": {discountType: "fixed", value: "9", minItems: 0, description: "$9 off your order"},
	"HAPPYHRS": {discountType: "percentage", value: "18", minItems: 0, description: "Happy Hours: 18% off"},
}

var defaultRule = codeRule{
	discountType: "percentage",
	value:        "10",
	minItems:     0,
	description:  "Valid promo code: 10% off",
}

// fileResult holds candidate codes found in a single file during pass 2.
type fileResult struct {
	candidates map[string]uint
}

func main() {
	var (
		dataDir     string
		databaseURL string
	)

	flag.StringVar(&dataDir, "data-dir", "data", "directory containing couponbaseN.gz files")
	flag.StringVar(&databaseURL, "database-url", "", "PostgreSQL connection URL (or DATABASE_URL env)")
	flag.Parse()

	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	if databaseURL == "" {
		slog.Error("database URL is required: set --database-url or DATABASE_URL")
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, dataDir, databaseURL); err != nil {
		slog.Error("coupon ingest failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	slog.Info("coupon ingest completed successfully")
}

func run(ctx context.Context, dataDir, databaseURL string) error {
	files := make([]string, numFiles)
	for i := range numFiles {
		files[i] = filepath.Join(dataDir, fmt.Sprintf("couponbase%d.gz", i+1))
	}
	for _, f := range files {
		if _, err := os.Stat(f); err != nil {
			return errors.Wrapf(err, "check file %s", f)
		}
	}

	// Pass 1: Build bloom filters concurrently.
	slog.Info("pass 1: building bloom filters", slog.Int("files", numFiles))

	filters, err := buildBloomFilters(ctx, files)
	if err != nil {
		return errors.Wrap(err, "build bloom filters")
	}

	// Pass 2: Find candidate codes appearing in 2+ files.
	slog.Info("pass 2: finding candidate codes")

	validCodes, err := findValidCodes(ctx, files, filters)
	if err != nil {
		return errors.Wrap(err, "find valid codes")
	}

	slog.Info("valid codes found", slog.Int("count", len(validCodes)))

	if len(validCodes) == 0 {
		slog.Info("no valid codes to insert")
		return nil
	}

	// Write valid codes to database.
	slog.Info("connecting to database")

	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		return errors.Wrap(err, "connect to database")
	}
	defer pool.Close()

	if err := writeCoupons(ctx, sqlc.New(pool), validCodes); err != nil {
		return errors.Wrap(err, "write coupons to database")
	}

	return nil
}

// buildBloomFilters creates one bloom filter per file, concurrently.
func buildBloomFilters(ctx context.Context, files []string) ([]*bloom.BloomFilter, error) {
	filters := make([]*bloom.BloomFilter, len(files))

	g, ctx := errgroup.WithContext(ctx)
	for i, f := range files {
		g.Go(buildFilterForFile(ctx, i, f, filters))
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return filters, nil
}

func buildFilterForFile(ctx context.Context, idx int, path string, filters []*bloom.BloomFilter) func() error {
	return func() error {
		filter := bloom.NewWithEstimates(bloomCapacity, bloomFPR)
		var count uint64

		if err := streamGzFile(ctx, path, func(code string) {
			if len(code) >= minCodeLen && len(code) <= maxCodeLen {
				filter.AddString(code)
				count++
				if count%progressEvery == 0 {
					slog.Info("pass 1 progress",
						slog.Int("file", idx+1),
						slog.Uint64("codes", count),
					)
				}
			}
		}); err != nil {
			return errors.Wrapf(err, "build filter for file %d", idx+1)
		}

		slog.Info("pass 1 complete",
			slog.Int("file", idx+1),
			slog.Uint64("total_codes", count),
		)

		filters[idx] = filter
		return nil
	}
}

// findValidCodes re-streams each file and checks codes against OTHER files' bloom filters.
// A code is valid if it appears in 2 or more files.
func findValidCodes(ctx context.Context, files []string, filters []*bloom.BloomFilter) ([]string, error) {
	results := make([]fileResult, len(files))

	g, ctx := errgroup.WithContext(ctx)
	for i, f := range files {
		g.Go(findCandidatesInFile(ctx, i, f, filters, results))
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Merge bitmasks from all files.
	merged := make(map[string]uint)
	for _, r := range results {
		for code, mask := range r.candidates {
			merged[code] |= mask
		}
	}

	// Keep codes appearing in 2+ files.
	var valid []string
	for code, mask := range merged {
		if bits.OnesCount(mask) >= 2 {
			valid = append(valid, code)
		}
	}

	return valid, nil
}

func findCandidatesInFile(
	ctx context.Context,
	idx int,
	path string,
	filters []*bloom.BloomFilter,
	results []fileResult,
) func() error {
	return func() error {
		candidates := make(map[string]uint)
		fileBit := uint(1) << uint(idx)
		var count uint64

		if err := streamGzFile(ctx, path, func(code string) {
			if len(code) < minCodeLen || len(code) > maxCodeLen {
				return
			}

			count++
			if count%progressEvery == 0 {
				slog.Info("pass 2 progress",
					slog.Int("file", idx+1),
					slog.Uint64("codes", count),
				)
			}

			// Check if this code appears in any OTHER file's bloom filter.
			for j, f := range filters {
				if j == idx {
					continue
				}
				if f.TestString(code) {
					candidates[code] |= fileBit
					break
				}
			}
		}); err != nil {
			return errors.Wrapf(err, "scan file %d for candidates", idx+1)
		}

		slog.Info("pass 2 complete",
			slog.Int("file", idx+1),
			slog.Uint64("total_codes", count),
			slog.Int("candidates", len(candidates)),
		)

		results[idx] = fileResult{candidates: candidates}
		return nil
	}
}

// streamGzFile opens a gzip-compressed file and calls fn for each line.
func streamGzFile(ctx context.Context, path string, fn func(code string)) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "open %s", path)
	}
	defer func() { _ = f.Close() }()

	gz, err := pgzip.NewReader(f)
	if err != nil {
		return errors.Wrapf(err, "create gzip reader for %s", path)
	}
	defer func() { _ = gz.Close() }()

	scanner := bufio.NewScanner(gz)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		fn(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "scan %s", path)
	}

	return nil
}

// writeCoupons upserts all valid coupon codes into the database.
func writeCoupons(ctx context.Context, queries *sqlc.Queries, codes []string) error {
	slog.Info("writing coupons to database", slog.Int("count", len(codes)))

	for i, code := range codes {
		rule, ok := codeRules[code]
		if !ok {
			rule = defaultRule
		}

		value, err := decimal.NewFromString(rule.value)
		if err != nil {
			return errors.Wrapf(err, "parse decimal value for code %s", code)
		}

		if err := queries.UpsertCoupon(ctx, sqlc.UpsertCouponParams{
			Code:         code,
			DiscountType: rule.discountType,
			Value:        value,
			MinItems:     rule.minItems,
			Description:  rule.description,
			Active:       true,
		}); err != nil {
			return errors.Wrapf(err, "upsert coupon %s", code)
		}

		if (i+1)%100 == 0 || i+1 == len(codes) {
			slog.Info("write progress", slog.Int("written", i+1), slog.Int("total", len(codes)))
		}
	}

	return nil
}
