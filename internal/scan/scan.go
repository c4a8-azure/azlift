// Package scan implements Stage 1 of the azlift pipeline: Azure Resource Graph
// inventory and cross-RG dependency analysis.
package scan

import "github.com/c4a8-azure/azlift/internal/logger"

// StageLabel is the logger stage identifier for the SCAN stage.
const StageLabel = logger.StageScan
