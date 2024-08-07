package db

import (
	"bytes"
	"fmt"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
	"github.com/techagentng/citizenx/models"
	"gorm.io/gorm"
)

const (
	DefaultPageSize = 20
	DefaultPage     = 1
)

type IncidentReportRepository interface {
	SaveBookmarkReport(bookmark *models.BookmarkReport) error
	SaveIncidentReport(report *models.IncidentReport) (*models.IncidentReport, error)
	HasPreviousReports(userID uint) (bool, error)
	UpdateReward(userID uint, reward *models.Reward) error
	FindUserByID(id uint) (*models.UserResponse, error)
	GetReportByID(report_id string) (*models.IncidentReport, error)
	CheckReportInBookmarkedReport(userID uint, reportID string) (bool, error)
	GetAllReports(page int) ([]models.IncidentReport, error)
	GetAllReportsByState(state string, page int) ([]models.IncidentReport, error)
	GetAllReportsByLGA(lga string, page int) ([]models.IncidentReport, error)
	GetAllReportsByReportType(lga string, page int) ([]models.IncidentReport, error)
	GetReportPercentageByState() ([]models.StateReportPercentage, error)
	Save(report *models.IncidentReport) error
	GetReportStatusByID(reportID string) (string, error)
	UpdateIncidentReport(report *models.IncidentReport) (*models.IncidentReport, error)
	GetReportsPostedTodayCount() (int64, error)
	GetTotalUserCount() (int64, error)
	GetRegisteredUsersCountByLGA(lga string) (int64, error)
	GetAllReportsByStateByTime(state string, startTime, endTime time.Time, page int) ([]models.IncidentReport, error)
	GetReportsByTypeAndLGA(reportType string, lga string) ([]models.SubReport, error)
	GetReportTypeCounts(state string, lga string, startDate, endDate *string) ([]string, []int, int, int, []models.StateReportCount, error)
	SaveStateLgaReportType(lga *models.LGA, state *models.State, reportType *models.ReportType, subReport *models.SubReport) error
	GetIncidentMarkers() ([]Marker, error)
	DeleteByID(id string) error
	GetStateReportCounts() ([]models.StateReportCount, error)
	GetVariadicStateReportCounts(reportTypes []string, states []string, startDate, endDate *time.Time) ([]models.StateReportCount, error)
	GetAllCategories() ([]string, error)
	GetAllStates() ([]string, error)
	GetRatingPercentages(reportType, state string) (*models.RatingPercentage, error)
	GetReportCountsByStateAndLGA() ([]models.ReportCount, error)
	ListAllStatesWithReportCounts() ([]models.StateReportCount, error)
	GetTotalReportCount() (int64, error)
	UploadFileToS3(s *session.Session, file multipart.File, fileName string, size int64) (string, error)
	GetNamesByCategory(stateName string, lgaID string, reportTypeCategory string) ([]string, error)
}

type incidentReportRepo struct {
	DB *gorm.DB
}

func NewIncidentReportRepo(db *GormDB) IncidentReportRepository {
	return &incidentReportRepo{db.DB}
}

func (i *incidentReportRepo) UpdateReward(userID uint, reward *models.Reward) error {
	// Find the existing reward for the user
	existingReward := &models.Reward{}

	// Retrieve the existing reward from the database
	if err := i.DB.Where("user_id = ?", userID).First(existingReward).Error; err != nil {
		// Check if the error is due to record not found
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// If record not found, create a new reward with the provided details
			// and save it to the database
			if err := i.DB.Create(reward).Error; err != nil {
				return err
			}
			return nil
		}
		// Return other errors
		return err
	}

	// Update the existing reward with the new values
	existingReward.RewardType = reward.RewardType
	existingReward.Point = reward.Point
	existingReward.IncidentReportID = reward.IncidentReportID

	// Update the balance if provided in the reward parameter
	if reward.Balance != 0 {
		existingReward.Balance = reward.Balance
	}

	// Save the updated reward to the database
	if err := i.DB.Save(existingReward).Error; err != nil {
		return err
	}

	return nil
}

func (i *incidentReportRepo) SaveIncidentReport(report *models.IncidentReport) (*models.IncidentReport, error) {
	// Save the new report to the database
	if err := i.DB.Create(&report).Error; err != nil {
		return nil, fmt.Errorf("failed to save report: %v", err)
	}

	return report, nil
}

func (i *incidentReportRepo) HasPreviousReports(userID uint) (bool, error) {
	// Retrieve the database connection from the GormDB struct
	db := i.DB

	// Initialize a reward object to store the query result
	var reward models.Reward

	// Query the database to find a reward for the given user ID where the amount is greater than 0
	err := db.Where("user_id = ? AND balance > ?", userID, 0).First(&reward).Error
	if err != nil {
		// If the error is "record not found", return false indicating no previous reports
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		// Return the error if it's not a "record not found" error
		return false, fmt.Errorf("could not find reward for user: %v", err)
	}

	// If the reward amount is greater than 0, return true indicating previous reports
	return true, nil
}

func (i *incidentReportRepo) FindUserByID(id uint) (*models.UserResponse, error) {
	var user models.UserResponse
	err := i.DB.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (i *incidentReportRepo) GetReportByID(report_id string) (*models.IncidentReport, error) {
	var report models.IncidentReport
	err := i.DB.Where("id = ?", report_id).First(&report).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &report, nil
}

func (repo *incidentReportRepo) CheckReportInBookmarkedReport(userID uint, reportID string) (bool, error) {
	var bookmarkedReport models.BookmarkReport
	if err := repo.DB.Where("user_id = ? AND report_id = ?", userID, reportID).First(&bookmarkedReport).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (repo *incidentReportRepo) SaveBookmarkReport(bookmark *models.BookmarkReport) error {
	tx := repo.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Create(bookmark).Error; err != nil {
		tx.Rollback()
		log.Printf("Error creating bookmark: %v", err)
		return err
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Error committing transaction: %v", err)
		return err
	}

	log.Printf("Bookmark saved successfully: %+v", bookmark)
	return nil
}

func (repo *incidentReportRepo) GetAllReports(page int) ([]models.IncidentReport, error) {
	var reports []models.IncidentReport
	// Calculate the offset
	offset := (page - 1) * 20

	err := repo.DB.Limit(20).Offset(offset).Order("timeof_incidence DESC").Find(&reports).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("no incident reports found")
		}
		return nil, err
	}
	return reports, nil
}

func (repo *incidentReportRepo) GetAllReportsByState(state string, page int) ([]models.IncidentReport, error) {
	var reports []models.IncidentReport
	offset := (page - 1) * DefaultPageSize

	err := repo.DB.Where("state = ?", state).
		Order("timeof_incidence DESC").
		Limit(DefaultPageSize).
		Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, err
	}
	return reports, nil
}

// GetAllReportsByState returns incident reports filtered by state and time range, with pagination
func (repo *incidentReportRepo) GetAllReportsByStateByTime(state string, startTime, endTime time.Time, page int) ([]models.IncidentReport, error) {
	var reports []models.IncidentReport
	offset := (page - 1) * DefaultPageSize

	err := repo.DB.Where("state = ? AND timeof_incidence BETWEEN ? AND ?", state, startTime, endTime).
		Order("timeof_incidence DESC").
		Limit(DefaultPageSize).
		Offset(offset).
		Find(&reports).Error

	if err != nil {
		return nil, err
	}
	return reports, nil
}

func (repo *incidentReportRepo) GetAllReportsByLGA(lga string, page int) ([]models.IncidentReport, error) {
	var reports []models.IncidentReport
	offset := (page - 1) * DefaultPageSize

	err := repo.DB.Where("lga = ?", lga).
		Order("timeof_incidence DESC").
		Limit(DefaultPageSize).
		Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, err
	}
	return reports, nil
}

func (repo *incidentReportRepo) GetAllReportsByReportType(reportType string, page int) ([]models.IncidentReport, error) {
	var reports []models.IncidentReport
	offset := (page - 1) * DefaultPageSize

	err := repo.DB.Where("report_type = ?", reportType).
		Order("timeof_incidence DESC").
		Limit(DefaultPageSize).
		Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, err
	}
	return reports, nil
}

func (r *incidentReportRepo) GetRewardByUserID(userID uint) (*models.Reward, error) {
	var reward models.Reward
	if err := r.DB.First(&reward, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &reward, nil
}

func (r *incidentReportRepo) Save(report *models.IncidentReport) error {
	return r.DB.Create(report).Error
}

func (r *incidentReportRepo) GetReportPercentageByState() ([]models.StateReportPercentage, error) {
	var results []models.StateReportPercentage

	query := `
        SELECT 
            state_name, 
            COUNT(*) AS count, 
            (COUNT(*) * 100.0 / (SELECT COUNT(*) FROM incident_reports)) AS percentage 
        FROM 
            incident_reports 
        GROUP BY 
            state_name;
    `

	if err := r.DB.Raw(query).Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

func (repo *incidentReportRepo) GetReportStatusByID(reportID string) (string, error) {
	var report models.IncidentReport
	err := repo.DB.Select("report_status").Where("id = ?", reportID).First(&report).Error
	if err != nil {
		return "", err
	}
	return report.ReportStatus, nil
}

func (i *incidentReportRepo) UpdateIncidentReport(report *models.IncidentReport) (*models.IncidentReport, error) {
	// Update the existing report in the database
	if err := i.DB.Save(report).Error; err != nil {
		return nil, fmt.Errorf("failed to update report: %v", err)
	}

	return report, nil
}

func (repo *incidentReportRepo) GetReportsPostedTodayCount() (int64, error) {
	var count int64
	// Get the start of today
	startOfToday := time.Now().Truncate(24 * time.Hour)

	// Count the reports posted today
	err := repo.DB.Model(&models.IncidentReport{}).
		Where("timeof_incidence >= ?", startOfToday).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetTotalUserCount returns the total number of users in the database
func (repo *incidentReportRepo) GetTotalUserCount() (int64, error) {
	var count int64

	// Count the total number of users
	err := repo.DB.Model(&models.User{}).Count(&count).Error
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetRegisteredUsersCountByLGA returns the number of registered users in a specific Local Government Area (LGA)
func (repo *incidentReportRepo) GetRegisteredUsersCountByLGA(lga string) (int64, error) {
	var count int64

	// Count the total number of users in the specified LGA
	err := repo.DB.Model(&models.User{}).
		Where("lga_name = ?", lga).
		Count(&count).Error

	if err != nil {
		log.Printf("Error querying database: %v", err)
		return 0, err
	}

	log.Printf("LGA: %s, User Count: %d", lga, count)
	return count, nil
}

func (repo *incidentReportRepo) GetReportsByTypeAndLGA(reportType string, lga string) ([]models.SubReport, error) {
	var reports []models.SubReport
	err := repo.DB.Joins("JOIN report_types ON report_types.id = sub_reports.report_type_id").
		Joins("JOIN lgas ON lgas.id = sub_reports.lga_id").
		Where("report_types.name = ? AND lgas.name = ?", reportType, lga).
		Find(&reports).Error
	if err != nil {
		return nil, err
	}
	return reports, nil
}

// GetReportTypeCounts gets the report types and their corresponding incident report counts

func (repo *incidentReportRepo) GetReportTypeCounts(state string, lga string, startDate, endDate *string) ([]string, []int, int, int, []models.StateReportCount, error) {
    var reportTypes []string
    var counts []int
    var totalUsers int
    var totalReports int
    var topStates []models.StateReportCount

    // Base query for report types and counts
    query := `
        SELECT rt.category, COUNT(*) AS count,
               (SELECT COUNT(DISTINCT rt.user_id) FROM report_types rt WHERE rt.state_name = ? AND rt.lga_name = ?) AS total_users,
               (SELECT COUNT(*) FROM report_types rt WHERE rt.state_name = ? AND rt.lga_name = ?) AS total_reports
        FROM report_types rt
        WHERE rt.state_name = ? AND rt.lga_name = ?
    `

    // Prepare query arguments
    var args []interface{}
    args = append(args, state, lga, state, lga, state, lga)

    // Optional date filter
    if startDate != nil && endDate != nil && *startDate != "" && *endDate != "" {
        var err error
        defaultStartDate, err := time.Parse("2006-01-02", *startDate)
        if err != nil {
            return nil, nil, 0, 0, nil, errors.New("failed to parse start date: " + err.Error())
        }

        defaultEndDate, err := time.Parse("2006-01-02", *endDate)
        if err != nil {
            return nil, nil, 0, 0, nil, errors.New("failed to parse end date: " + err.Error())
        }

        query += ` AND rt.date_of_incidence BETWEEN ? AND ?`
        args = append(args, defaultStartDate, defaultEndDate)
    }

    query += ` GROUP BY rt.category`

    // Execute the query with parameters
    rows, err := repo.DB.Raw(query, args...).Rows()
    if err != nil {
        return nil, nil, 0, 0, nil, err
    }
    defer rows.Close()

    // Process the result rows
    for rows.Next() {
        var reportType string
        var count int
        if err := rows.Scan(&reportType, &count, &totalUsers, &totalReports); err != nil {
            return nil, nil, 0, 0, nil, err
        }
        reportTypes = append(reportTypes, reportType)
        counts = append(counts, count)
    }

    if err := rows.Err(); err != nil {
        return nil, nil, 0, 0, nil, err
    }

    // Query to get all states with report counts
    topStatesQuery := `
        SELECT state_name, COUNT(*) AS report_count
        FROM report_types
        WHERE lga_name = ?
    `

    // Append date filters if provided
    if startDate != nil && endDate != nil && *startDate != "" && *endDate != "" {
        topStatesQuery += ` AND date_of_incidence BETWEEN ? AND ?`
    }

    topStatesQuery += `
        GROUP BY state_name
        ORDER BY report_count DESC
    `

    topStatesArgs := []interface{}{lga}
    if startDate != nil && endDate != nil && *startDate != "" && *endDate != "" {
        defaultStartDate, err := time.Parse("2006-01-02", *startDate)
        if err != nil {
            return nil, nil, 0, 0, nil, errors.New("failed to parse start date: " + err.Error())
        }

        defaultEndDate, err := time.Parse("2006-01-02", *endDate)
        if err != nil {
            return nil, nil, 0, 0, nil, errors.New("failed to parse end date: " + err.Error())
        }

        topStatesArgs = append(topStatesArgs, defaultStartDate, defaultEndDate)
    }

    err = repo.DB.Raw(topStatesQuery, topStatesArgs...).Scan(&topStates).Error
    if err != nil {
        return nil, nil, 0, 0, nil, fmt.Errorf("could not fetch top states: %v", err)
    }

    return reportTypes, counts, totalUsers, totalReports, topStates, nil
}

// SaveReportTypeAndSubReport saves both ReportType and SubReport in a transaction
func (repo *incidentReportRepo) SaveStateLgaReportType(lga *models.LGA, state *models.State, reportType *models.ReportType, subReport *models.SubReport) error {
	// Start a transaction
	tx := repo.DB.Begin()

	// Commit ReportType to the database
	if err := tx.Create(reportType).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Commit SubReport to the database
	if err := tx.Create(subReport).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Create(lga).Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Create(state).Error; err != nil {
		tx.Rollback()
		return err
	}
	// Commit the transaction
	return tx.Commit().Error
}

type Marker struct {
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Popup string  `json:"popup"`
}

func (repo *incidentReportRepo) GetIncidentMarkers() ([]Marker, error) {
	var markers []Marker

	query := `
        SELECT
            latitude AS lat,
            longitude AS lng,
            incident_reports.state_name AS popup,
            COALESCE(report_counts.count, 0) AS count
        FROM
            incident_reports
        LEFT JOIN (
            SELECT
                state_name,
                COUNT(*) AS count
            FROM
                incident_reports
            GROUP BY
                state_name
        ) AS report_counts ON incident_reports.state_name = report_counts.state_name
        GROUP BY
            incident_reports.state_name, latitude, longitude, report_counts.count
    `

	if err := repo.DB.Raw(query).Scan(&markers).Error; err != nil {
		return nil, err
	}

	return markers, nil
}

func (repo *incidentReportRepo) DeleteByID(id string) error {
	var report models.SubReport
	if err := repo.DB.Where("id = ?", id).First(&report).Error; err != nil {
		return err
	}

	if err := repo.DB.Delete(&report).Error; err != nil {
		return err
	}

	return nil
}

func (repo *incidentReportRepo) GetStateReportCounts() ([]models.StateReportCount, error) {
	var stateReportCounts []models.StateReportCount

	err := repo.DB.Table("report_types").
		Select("state_name, COUNT(id) as report_count").
		Group("state_name").
		Scan(&stateReportCounts).Error

	if err != nil {
		return nil, err
	}

	return stateReportCounts, nil
}

func (repo *incidentReportRepo) GetVariadicStateReportCounts(reportTypes []string, states []string, startDate, endDate *time.Time) ([]models.StateReportCount, error) {
	var stateReportCounts []models.StateReportCount

	// Initialize the query on ReportType model
	db := repo.DB.Model(&models.ReportType{})

	// Select state_name, category, and count the reports, grouping by state_name and category
	query := db.Select("state_name, category, COUNT(id) as report_count").Group("state_name, category")

	// Add report type filter if provided
	if len(reportTypes) > 0 {
		query = query.Where("category IN (?)", reportTypes)
	}

	// Add state filters if provided
	if len(states) > 0 {
		query = query.Where("state_name IN (?)", states)
	}

	// Add date range filter if both dates are provided
	if startDate != nil && endDate != nil {
		query = query.Where("date_of_incidence BETWEEN ? AND ?", startDate, endDate)
	} else if startDate != nil {
		query = query.Where("date_of_incidence >= ?", startDate)
	} else if endDate != nil {
		query = query.Where("date_of_incidence <= ?", endDate)
	}

	// Add filter to exclude empty state names
	query = query.Where("state_name <> ''")

	// Debugging: Log the final query
	sql, args := query.Statement.SQL.String(), query.Statement.Vars
	log.Printf("Final query: %s with args: %v", sql, args)

	// Execute the query and scan the results into stateReportCounts
	err := query.Scan(&stateReportCounts).Error
	if err != nil {
		return nil, err
	}

	return stateReportCounts, nil
}

func (i *incidentReportRepo) GetAllCategories() ([]string, error) {
	var categories []string
	if err := i.DB.Model(&models.ReportType{}).Distinct().Pluck("category", &categories).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch categories: %v", err)
	}
	return categories, nil
}

func (i *incidentReportRepo) GetAllStates() ([]string, error) {
	var states []string
	if err := i.DB.Model(&models.ReportType{}).Distinct().Pluck("state_name", &states).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch categories: %v", err)
	}
	return states, nil
}

func (i *incidentReportRepo) GetRatingPercentages(reportType, state string) (*models.RatingPercentage, error) {
	var totalCount int64
	var goodCount int64
	var badCount int64

	// Count total reports
	if err := i.DB.Model(&models.ReportType{}).
		Where("category = ? AND state_name = ?", reportType, state).
		Count(&totalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch total count: %v", err)
	}

	// Count good ratings
	if err := i.DB.Model(&models.ReportType{}).
		Where("category = ? AND state_name = ? AND incident_report_rating = ?", reportType, state, "good").
		Count(&goodCount).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch good count: %v", err)
	}

	// Count bad ratings
	if err := i.DB.Model(&models.ReportType{}).
		Where("category = ? AND state_name = ? AND incident_report_rating = ?", reportType, state, "bad").
		Count(&badCount).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch bad count: %v", err)
	}

	// Calculate percentages
	goodPercentage := float64(goodCount) / float64(totalCount) * 100
	badPercentage := float64(badCount) / float64(totalCount) * 100

	return &models.RatingPercentage{
		GoodPercentage: goodPercentage,
		BadPercentage:  badPercentage,
	}, nil
}

func (i *incidentReportRepo) GetReportCountsByStateAndLGA() ([]models.ReportCount, error) {
    var results []models.ReportCount

    err := i.DB.Model(&models.ReportType{}).
        Select("state_name, lga_name, COUNT(*) as count").
        Group("state_name, lga_name").
        Scan(&results).Error

    if err != nil {
        return nil, err
    }

    return results, nil
}

func (repo *incidentReportRepo) ListAllStatesWithReportCounts() ([]models.StateReportCount, error) {
    var topStates []models.StateReportCount

    // Query to get the top 6 states with their report counts
    query := `
        SELECT state_name, COUNT(*) AS report_count
        FROM report_types
        GROUP BY state_name
        ORDER BY report_count DESC
        LIMIT 6
    `

    // Execute the query
    err := repo.DB.Raw(query).Scan(&topStates).Error
    if err != nil {
        return nil, fmt.Errorf("could not fetch states with report counts: %v", err)
    }

    return topStates, nil
}

func (i *incidentReportRepo) GetTotalReportCount() (int64, error) {
    var count int64

    err := i.DB.Model(&models.ReportType{}).
        Count(&count).Error

    if err != nil {
        return 0, err
    }

    return count, nil
}

func (i *incidentReportRepo) UploadFileToS3(s *session.Session, file multipart.File, fileName string, size int64) (string, error) {
	// get the file size and read
	// the file content into a buffer
	buffer := make([]byte, size)
	file.Read(buffer)
	// config settings: this is where you choose the bucket,
	// filename, content-type and storage class of the file
	// you're uploading
	url := "https://s3-eu-west-3.amazonaws.com/arp-rental/" + fileName
	_, err := s3.New(s).PutObject(&s3.PutObjectInput{
		Bucket:               aws.String(os.Getenv("S3_BUCKET_NAME")),
		Key:                  aws.String(fileName),
		ACL:                  aws.String("public-read"),
		Body:                 bytes.NewReader(buffer),
		ContentLength:        aws.Int64(int64(size)),
		ContentType:          aws.String(http.DetectContentType(buffer)),
		ContentDisposition:   aws.String("attachment"),
		ServerSideEncryption: aws.String("AES256"),
		StorageClass:         aws.String("INTELLIGENT_TIERING"),
	})
	return url, err
}

func (i *incidentReportRepo) GetNamesByCategory(stateName string, lgaID string, reportTypeCategory string) ([]string, error) {
    var names []string

    err := i.DB.Model(&models.SubReport{}).
        Where("state_name = ? AND lga_id = ? AND report_type_category = ?", stateName, lgaID, reportTypeCategory).
        Pluck("sub_report_type", &names).Error

    if err != nil {
        return nil, err
    }

    return names, nil
}



