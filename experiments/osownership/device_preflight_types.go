//go:build mnet_enrollment_development

package osownership

type DeviceMetadata struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Rdev   uint64 `json:"rdev"`
	Mode   uint32 `json:"mode"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
}
type DevicePreflightEvidence struct {
	Version              int             `json:"version"`
	Path                 string          `json:"path"`
	Namespace            Namespace       `json:"namespace"`
	Metadata             *DeviceMetadata `json:"metadata,omitempty"`
	OpenedMetadata       *DeviceMetadata `json:"opened_metadata,omitempty"`
	Outcome              string          `json:"outcome"`
	Errno                int             `json:"errno,omitempty"`
	FD                   int             `json:"fd"`
	OpenAttempted        bool            `json:"open_attempted"`
	CloseAttempted       bool            `json:"close_attempted"`
	Closed               bool            `json:"closed"`
	CLOEXEC              bool            `json:"cloexec"`
	Nonblock             bool            `json:"nonblock"`
	FDsBefore            []int           `json:"fds_before"`
	FDsAfter             []int           `json:"fds_after"`
	FDInventoryUnchanged bool            `json:"fd_inventory_unchanged"`
	BaselineHash         string          `json:"baseline_hash"`
	FinalHash            string          `json:"final_hash"`
	BaselineUnchanged    bool            `json:"baseline_unchanged"`
	NamespaceRechecked   bool            `json:"namespace_rechecked"`
}
