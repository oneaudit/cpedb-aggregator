package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/oneaudit/cpedb-aggregator/pkg/types"
	"github.com/projectdiscovery/ratelimit"
	"github.com/remeh/sizedwaitgroup"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// https://nvd.nist.gov/developers/products
const (
	ApiEndpoint    = "https://services.nvd.nist.gov/rest/json/cpes/2.0/"
	ApiKeyEnvVar   = "CPEDB_API_KEY"
	ResultsPerPage = 10000
	RateLimit      = 50

	IsTest             = false
	TestOffset         = 10
	TestResultsPerPage = 10
	TestDoUpdateFile   = false

	UpdateFile      = ".update_date"
	ISO8601Format   = "2006-01-02T15:04:05Z"
	MaxIntervalDate = (120 - 1) * 24 * time.Hour
)

var (
	nameSanitize = regexp.MustCompile(`[^\w-.]`)
	rateLimiter  = ratelimit.New(context.Background(), RateLimit, time.Minute)
)

func main() {
	wg := sizedwaitgroup.New(10)

	// Retrieve the API key from GitHub Actions secrets
	apiKey := os.Getenv(ApiKeyEnvVar)
	if apiKey == "" {
		fmt.Println("API key is missing!")
		return
	}

	lastRunTime := time.Unix(0, 0)
	now := time.Now()
	if _, err := os.Stat(UpdateFile); err == nil {
		fileContent, err := os.ReadFile(UpdateFile)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", UpdateFile, err)
			return
		}

		lastRunTimestamp, err := strconv.ParseInt(string(fileContent), 10, 64)
		if err != nil {
			fmt.Printf("Error parsing timestamp from file: %v\n", err)
			return
		}
		lastRunTime = time.Unix(lastRunTimestamp, 0)
	} else {
		now = lastRunTime
	}

	var products []types.NistProduct
	var mu sync.Mutex
	if lastRunTime == now || IsTest {
		noDate := lastRunTime.Format(ISO8601Format)
		products = append(products, fetchCPEs(noDate, noDate, apiKey)...)
	} else {
		for start := lastRunTime; start.Before(now); start = start.Add(MaxIntervalDate) {
			end := start.Add(MaxIntervalDate)
			if end.After(now) {
				end = now
			}
			wg.Add()
			go func(startDate string, endDate string) {
				// Fetch and parse CPEs
				defer wg.Done()
				results := fetchCPEs(startDate, endDate, apiKey)
				mu.Lock()
				products = append(products, results...)
				mu.Unlock()
			}(start.Format(ISO8601Format), end.Format(ISO8601Format))
		}
	}
	wg.Wait()

	groupedProducts := make(map[string]map[string]*types.AggregatorResult)
	for _, nistProduct := range products {
		cpe := nistProduct.CPE.CPEName
		vendorDir, jsonPath, err := computeJsonFilePath(cpe)
		if err != nil {
			fmt.Printf("Skipping invalid CPE: %s\n", cpe)
			continue
		}
		// Group by vendor
		if _, exists := groupedProducts[vendorDir]; !exists {
			groupedProducts[vendorDir] = make(map[string]*types.AggregatorResult)
		}
		// Group by product (json file)
		if _, exists := groupedProducts[vendorDir][jsonPath]; !exists {
			groupedProducts[vendorDir][jsonPath] = &types.AggregatorResult{}
		}
		product := groupedProducts[vendorDir][jsonPath]
		product.Nist = append(product.Nist, nistProduct)
		product.Opencpe = append(product.Opencpe, buildOpenCpeProduct(nistProduct))
	}

	for vendorDir, vendorProducts := range groupedProducts {
		err := os.MkdirAll(vendorDir, 0755)
		if err != nil {
			fmt.Printf("error creating directory: %v", err)
			return
		}
		for jsonFilePath, vendorProduct := range vendorProducts {
			var finalProduct *types.AggregatorResult
			file, err := os.OpenFile(jsonFilePath, os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				fmt.Printf("error opening JSON file: %v\n", err)
				return
			}
			info, err := file.Stat()
			if err != nil {
				file.Close()
				fmt.Printf("error stating file: %v\n", err)
				return
			}
			if info.Size() > 0 {
				var existingProduct types.AggregatorResult
				decoder := json.NewDecoder(file)
				err = decoder.Decode(&existingProduct)
				if err != nil {
					file.Close()
					fmt.Printf("error decoding existing JSON file: %v\n", err)
					return
				}
				finalProduct = MergeAggregatorResults(vendorProduct, &existingProduct)
			} else {
				finalProduct = vendorProduct
			}

			err = file.Truncate(0)
			if err != nil {
				file.Close()
				fmt.Printf("error truncating file: %v\n", err)
				return
			}
			_, err = file.Seek(0, 0)
			if err != nil {
				file.Close()
				fmt.Printf("error seeking to file start: %v\n", err)
				return
			}

			encoder := json.NewEncoder(file)
			encoder.SetIndent("", "  ")
			err = encoder.Encode(finalProduct)
			file.Close()
			if err != nil {
				fmt.Printf("error writing to JSON file: %v\n", err)
				return
			}
		}
	}

	if !IsTest || TestDoUpdateFile {
		err := os.WriteFile(UpdateFile, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0644)
		if err != nil {
			fmt.Printf("Error writing to %s: %v\n", UpdateFile, err)
		}
	}
}

func MergeAggregatorResults(newResult *types.AggregatorResult, oldResult *types.AggregatorResult) *types.AggregatorResult {
	finalResult := types.AggregatorResult{}

	for _, oldNistProduct := range oldResult.Nist {
		var found *types.NistProduct
		for _, newNistProduct := range newResult.Nist {
			if oldNistProduct.CPE.CPENameID == newNistProduct.CPE.CPENameID {
				found = &newNistProduct
				break
			}
		}
		if found == nil {
			// the product did not change
			finalResult.Nist = append(finalResult.Nist, oldNistProduct)
		} else {
			// the product changed
			finalResult.Nist = append(finalResult.Nist, *found)
		}
	}

	for _, oldOpenCpeProduct := range oldResult.Opencpe {
		var found *types.OpenCPEProduct
		for _, newNistProduct := range newResult.Opencpe {
			if oldOpenCpeProduct.Name == newNistProduct.Name {
				found = &newNistProduct
				break
			}
		}
		if found == nil {
			// the product did not change
			finalResult.Opencpe = append(finalResult.Opencpe, oldOpenCpeProduct)
		} else {
			// the product changed
			finalResult.Opencpe = append(finalResult.Opencpe, *found)
		}
	}
	return &finalResult
}

func fetchCPEs(startDate string, endDate string, apiKey string) []types.NistProduct {
	var nistProducts []types.NistProduct
	offset := 0
	headers := map[string]string{
		"apiKey": apiKey,
	}
	var resultsPerPage int
	if IsTest {
		resultsPerPage = TestResultsPerPage
	} else {
		resultsPerPage = ResultsPerPage
	}

	for {
		if IsTest && offset >= TestOffset {
			break
		}

		// respect the limit rate
		rateLimiter.Take()

		var url string
		if startDate != endDate {
			url = fmt.Sprintf("%s?lastModStartDate=%s&lastModEndDate=%s&resultsPerPage=%d&startIndex=%d", ApiEndpoint, startDate, endDate, resultsPerPage, offset)
		} else {
			url = fmt.Sprintf("%s?resultsPerPage=%d&startIndex=%d", ApiEndpoint, resultsPerPage, offset)
		}

		fmt.Println("Fetching CPEs at...", url)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Printf("Error creating request: %v", err)
			return nil
		}

		for key, value := range headers {
			req.Header.Add(key, value)
		}

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error making the request: %v", err)
			return nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			fmt.Printf("Error: Received non-200 response code %d", resp.StatusCode)
			return nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("Error reading response body: %v", err)
			return nil
		}

		var apiResponse types.Response
		err = json.Unmarshal(body, &apiResponse)
		if err != nil {
			fmt.Printf("Error parsing JSON response: %v", err)
			return nil
		}

		nistProducts = append(nistProducts, apiResponse.Products...)
		offset += resultsPerPage

		if offset >= apiResponse.TotalResults {
			break
		}
	}

	return nistProducts
}

func computeJsonFilePath(cpe string) (string, string, error) {
	parts := strings.Split(cpe, ":")
	if len(parts) < 5 {
		return "", "", fmt.Errorf("invalid CPE string format")
	}
	vendorPart := sanitize(parts[3], "_")
	productPart := sanitize(parts[4], "_")
	vendorDir := filepath.Join(".", vendorPart)
	jsonFilePath := filepath.Join(vendorDir, productPart+".json")
	return vendorDir, jsonFilePath, nil
}

func sanitize(str string, newChar string) string {
	return strings.ReplaceAll(nameSanitize.ReplaceAllString(str, newChar), "./", newChar+"/")
}

func buildOpenCpeProduct(product types.NistProduct) types.OpenCPEProduct {
	opencpe := types.OpenCPEProduct{
		Name:       product.CPE.CPEName,
		Deprecated: product.CPE.Deprecated,
	}
	for _, title := range product.CPE.Titles {
		if opencpe.Title == "" || title.Lang == "en" {
			opencpe.Title = title.Title
		}
	}
	if opencpe.Deprecated && len(product.CPE.DeprecatedBy) > 0 {
		opencpe.DeprecatedOver = product.CPE.DeprecatedBy[0].CPEName
	}
	return opencpe
}
