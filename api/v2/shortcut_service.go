package v2

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mssola/useragent"
	"github.com/pkg/errors"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	apiv2pb "github.com/yourselfhosted/slash/proto/gen/api/v2"
	storepb "github.com/yourselfhosted/slash/proto/gen/store"
	"github.com/yourselfhosted/slash/server/metric"
	"github.com/yourselfhosted/slash/store"
)

func (s *APIV2Service) ListShortcuts(ctx context.Context, _ *apiv2pb.ListShortcutsRequest) (*apiv2pb.ListShortcutsResponse, error) {
	userID := ctx.Value(userIDContextKey).(int32)
	find := &store.FindShortcut{}
	find.VisibilityList = []store.Visibility{store.VisibilityWorkspace, store.VisibilityPublic}
	visibleShortcutList, err := s.Store.ListShortcuts(ctx, find)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch visible shortcut list, err: %v", err)
	}

	find.VisibilityList = []store.Visibility{store.VisibilityPrivate}
	find.CreatorID = &userID
	shortcutList, err := s.Store.ListShortcuts(ctx, find)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch private shortcut list, err: %v", err)
	}

	shortcutList = append(shortcutList, visibleShortcutList...)
	shortcuts := []*apiv2pb.Shortcut{}
	for _, shortcut := range shortcutList {
		composedShortcut, err := s.convertShortcutFromStorepb(ctx, shortcut)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to convert shortcut, err: %v", err)
		}
		shortcuts = append(shortcuts, composedShortcut)
	}

	response := &apiv2pb.ListShortcutsResponse{
		Shortcuts: shortcuts,
	}
	return response, nil
}

func (s *APIV2Service) GetShortcut(ctx context.Context, request *apiv2pb.GetShortcutRequest) (*apiv2pb.GetShortcutResponse, error) {
	shortcut, err := s.Store.GetShortcut(ctx, &store.FindShortcut{
		Name: &request.Name,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get shortcut by name: %v", err)
	}
	if shortcut == nil {
		return nil, status.Errorf(codes.NotFound, "shortcut not found")
	}

	userID, ok := ctx.Value(userIDContextKey).(int32)
	if ok {
		if shortcut.Visibility == storepb.Visibility_PRIVATE && shortcut.CreatorId != userID {
			return nil, status.Errorf(codes.PermissionDenied, "Permission denied")
		}
	} else {
		if shortcut.Visibility != storepb.Visibility_PUBLIC {
			return nil, status.Errorf(codes.PermissionDenied, "Permission denied")
		}
	}

	// Create activity if request is from record view.
	if request.RecordView {
		if err := s.createShortcutViewActivity(ctx, shortcut); err != nil {
			fmt.Printf("failed to create activity, err: %v", err)
		}
	}

	composedShortcut, err := s.convertShortcutFromStorepb(ctx, shortcut)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert shortcut, err: %v", err)
	}
	response := &apiv2pb.GetShortcutResponse{
		Shortcut: composedShortcut,
	}
	return response, nil
}

func (s *APIV2Service) GetShortcutById(ctx context.Context, request *apiv2pb.GetShortcutByIdRequest) (*apiv2pb.GetShortcutByIdResponse, error) {
	shortcut, err := s.Store.GetShortcut(ctx, &store.FindShortcut{
		ID: &request.Id,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get shortcut by id: %v", err)
	}
	if shortcut == nil {
		return nil, status.Errorf(codes.NotFound, "shortcut not found")
	}

	userID, ok := ctx.Value(userIDContextKey).(int32)
	if ok {
		if shortcut.Visibility == storepb.Visibility_PRIVATE && shortcut.CreatorId != userID {
			return nil, status.Errorf(codes.PermissionDenied, "Permission denied")
		}
	} else {
		if shortcut.Visibility != storepb.Visibility_PUBLIC {
			return nil, status.Errorf(codes.PermissionDenied, "Permission denied")
		}
	}

	composedShortcut, err := s.convertShortcutFromStorepb(ctx, shortcut)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert shortcut, err: %v", err)
	}
	response := &apiv2pb.GetShortcutByIdResponse{
		Shortcut: composedShortcut,
	}
	return response, nil
}

func (s *APIV2Service) CreateShortcut(ctx context.Context, request *apiv2pb.CreateShortcutRequest) (*apiv2pb.CreateShortcutResponse, error) {
	userID := ctx.Value(userIDContextKey).(int32)
	shortcut := &storepb.Shortcut{
		CreatorId:   userID,
		Name:        request.Shortcut.Name,
		Link:        request.Shortcut.Link,
		Title:       request.Shortcut.Title,
		Tags:        request.Shortcut.Tags,
		Description: request.Shortcut.Description,
		Visibility:  storepb.Visibility(request.Shortcut.Visibility),
		OgMetadata:  &storepb.OpenGraphMetadata{},
	}
	if request.Shortcut.OgMetadata != nil {
		shortcut.OgMetadata = &storepb.OpenGraphMetadata{
			Title:       request.Shortcut.OgMetadata.Title,
			Description: request.Shortcut.OgMetadata.Description,
			Image:       request.Shortcut.OgMetadata.Image,
		}
	}
	shortcut, err := s.Store.CreateShortcut(ctx, shortcut)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create shortcut, err: %v", err)
	}
	if err := s.createShortcutCreateActivity(ctx, shortcut); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create activity, err: %v", err)
	}

	composedShortcut, err := s.convertShortcutFromStorepb(ctx, shortcut)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert shortcut, err: %v", err)
	}
	response := &apiv2pb.CreateShortcutResponse{
		Shortcut: composedShortcut,
	}
	return response, nil
}

func (s *APIV2Service) UpdateShortcut(ctx context.Context, request *apiv2pb.UpdateShortcutRequest) (*apiv2pb.UpdateShortcutResponse, error) {
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "updateMask is required")
	}

	userID := ctx.Value(userIDContextKey).(int32)
	currentUser, err := s.Store.GetUser(ctx, &store.FindUser{
		ID: &userID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user, err: %v", err)
	}
	shortcut, err := s.Store.GetShortcut(ctx, &store.FindShortcut{
		ID: &request.Shortcut.Id,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get shortcut by name: %v", err)
	}
	if shortcut == nil {
		return nil, status.Errorf(codes.NotFound, "shortcut not found")
	}
	if shortcut.CreatorId != userID && currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "Permission denied")
	}

	update := &store.UpdateShortcut{
		ID: shortcut.Id,
	}
	for _, path := range request.UpdateMask.Paths {
		switch path {
		case "name":
			update.Name = &request.Shortcut.Name
		case "link":
			update.Link = &request.Shortcut.Link
		case "title":
			update.Title = &request.Shortcut.Title
		case "description":
			update.Description = &request.Shortcut.Description
		case "tags":
			tag := strings.Join(request.Shortcut.Tags, " ")
			update.Tag = &tag
		case "visibility":
			visibility := store.Visibility(request.Shortcut.Visibility.String())
			update.Visibility = &visibility
		case "og_metadata":
			if request.Shortcut.OgMetadata != nil {
				update.OpenGraphMetadata = &storepb.OpenGraphMetadata{
					Title:       request.Shortcut.OgMetadata.Title,
					Description: request.Shortcut.OgMetadata.Description,
					Image:       request.Shortcut.OgMetadata.Image,
				}
			}
		}
	}
	shortcut, err = s.Store.UpdateShortcut(ctx, update)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update shortcut, err: %v", err)
	}

	composedShortcut, err := s.convertShortcutFromStorepb(ctx, shortcut)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to convert shortcut, err: %v", err)
	}
	response := &apiv2pb.UpdateShortcutResponse{
		Shortcut: composedShortcut,
	}
	return response, nil
}

func (s *APIV2Service) DeleteShortcut(ctx context.Context, request *apiv2pb.DeleteShortcutRequest) (*apiv2pb.DeleteShortcutResponse, error) {
	userID := ctx.Value(userIDContextKey).(int32)
	currentUser, err := s.Store.GetUser(ctx, &store.FindUser{
		ID: &userID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user, err: %v", err)
	}
	shortcut, err := s.Store.GetShortcut(ctx, &store.FindShortcut{
		Name: &request.Name,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get shortcut by name: %v", err)
	}
	if shortcut == nil {
		return nil, status.Errorf(codes.NotFound, "shortcut not found")
	}
	if shortcut.CreatorId != userID && currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "Permission denied")
	}

	err = s.Store.DeleteShortcut(ctx, &store.DeleteShortcut{
		ID: shortcut.Id,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete shortcut, err: %v", err)
	}
	response := &apiv2pb.DeleteShortcutResponse{}
	return response, nil
}

func (s *APIV2Service) GetShortcutAnalytics(ctx context.Context, request *apiv2pb.GetShortcutAnalyticsRequest) (*apiv2pb.GetShortcutAnalyticsResponse, error) {
	shortcut, err := s.Store.GetShortcut(ctx, &store.FindShortcut{
		Name: &request.Name,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get shortcut by name: %v", err)
	}
	if shortcut == nil {
		return nil, status.Errorf(codes.NotFound, "shortcut not found")
	}

	activities, err := s.Store.ListActivities(ctx, &store.FindActivity{
		Type:  store.ActivityShortcutView,
		Where: []string{fmt.Sprintf("json_extract(payload, '$.shortcutId') = %d", shortcut.Id)},
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get activities, err: %v", err)
	}

	referenceMap := make(map[string]int32)
	deviceMap := make(map[string]int32)
	browserMap := make(map[string]int32)
	for _, activity := range activities {
		payload := &storepb.ActivityShorcutViewPayload{}
		if err := protojson.Unmarshal([]byte(activity.Payload), payload); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("failed to unmarshal payload, err: %v", err))
		}

		if _, ok := referenceMap[payload.Referer]; !ok {
			referenceMap[payload.Referer] = 0
		}
		referenceMap[payload.Referer]++

		ua := useragent.New(payload.UserAgent)
		deviceName := ua.OSInfo().Name
		browserName, _ := ua.Browser()

		if _, ok := deviceMap[deviceName]; !ok {
			deviceMap[deviceName] = 0
		}
		deviceMap[deviceName]++

		if _, ok := browserMap[browserName]; !ok {
			browserMap[browserName] = 0
		}
		browserMap[browserName]++
	}

	metric.Enqueue("shortcut analytics")
	response := &apiv2pb.GetShortcutAnalyticsResponse{
		References: mapToAnalyticsSlice(referenceMap),
		Devices:    mapToAnalyticsSlice(deviceMap),
		Browsers:   mapToAnalyticsSlice(browserMap),
	}
	return response, nil
}

func mapToAnalyticsSlice(m map[string]int32) []*apiv2pb.GetShortcutAnalyticsResponse_AnalyticsItem {
	analyticsSlice := make([]*apiv2pb.GetShortcutAnalyticsResponse_AnalyticsItem, 0)
	for key, value := range m {
		analyticsSlice = append(analyticsSlice, &apiv2pb.GetShortcutAnalyticsResponse_AnalyticsItem{
			Name:  key,
			Count: value,
		})
	}
	slices.SortFunc(analyticsSlice, func(i, j *apiv2pb.GetShortcutAnalyticsResponse_AnalyticsItem) int {
		return int(i.Count - j.Count)
	})
	return analyticsSlice
}

func (s *APIV2Service) createShortcutViewActivity(ctx context.Context, shortcut *storepb.Shortcut) error {
	p, _ := peer.FromContext(ctx)
	headers, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return errors.New("Failed to get metadata from context")
	}
	payload := &storepb.ActivityShorcutViewPayload{
		ShortcutId: shortcut.Id,
		Ip:         p.Addr.String(),
		Referer:    headers.Get("referer")[0],
		UserAgent:  headers.Get("user-agent")[0],
	}
	payloadStr, err := protojson.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "Failed to marshal activity payload")
	}
	activity := &store.Activity{
		CreatorID: BotID,
		Type:      store.ActivityShortcutCreate,
		Level:     store.ActivityInfo,
		Payload:   string(payloadStr),
	}
	_, err = s.Store.CreateActivity(ctx, activity)
	if err != nil {
		return errors.Wrap(err, "Failed to create activity")
	}
	return nil
}

func (s *APIV2Service) createShortcutCreateActivity(ctx context.Context, shortcut *storepb.Shortcut) error {
	payload := &storepb.ActivityShorcutCreatePayload{
		ShortcutId: shortcut.Id,
	}
	payloadStr, err := protojson.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "Failed to marshal activity payload")
	}
	activity := &store.Activity{
		CreatorID: shortcut.CreatorId,
		Type:      store.ActivityShortcutCreate,
		Level:     store.ActivityInfo,
		Payload:   string(payloadStr),
	}
	_, err = s.Store.CreateActivity(ctx, activity)
	if err != nil {
		return errors.Wrap(err, "Failed to create activity")
	}
	return nil
}

func (s *APIV2Service) convertShortcutFromStorepb(ctx context.Context, shortcut *storepb.Shortcut) (*apiv2pb.Shortcut, error) {
	composedShortcut := &apiv2pb.Shortcut{
		Id:          shortcut.Id,
		CreatorId:   shortcut.CreatorId,
		CreatedTime: timestamppb.New(time.Unix(shortcut.CreatedTs, 0)),
		UpdatedTime: timestamppb.New(time.Unix(shortcut.UpdatedTs, 0)),
		RowStatus:   apiv2pb.RowStatus(shortcut.RowStatus),
		Name:        shortcut.Name,
		Link:        shortcut.Link,
		Title:       shortcut.Title,
		Tags:        shortcut.Tags,
		Description: shortcut.Description,
		Visibility:  apiv2pb.Visibility(shortcut.Visibility),
		OgMetadata: &apiv2pb.OpenGraphMetadata{
			Title:       shortcut.OgMetadata.Title,
			Description: shortcut.OgMetadata.Description,
			Image:       shortcut.OgMetadata.Image,
		},
	}

	activityList, err := s.Store.ListActivities(ctx, &store.FindActivity{
		Type:  store.ActivityShortcutView,
		Level: store.ActivityInfo,
		Where: []string{fmt.Sprintf("json_extract(payload, '$.shortcutId') = %d", composedShortcut.Id)},
	})
	if err != nil {
		return nil, errors.Wrap(err, "Failed to list activities")
	}
	composedShortcut.ViewCount = int32(len(activityList))

	return composedShortcut, nil
}
