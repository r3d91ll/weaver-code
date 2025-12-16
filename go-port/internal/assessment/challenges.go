// Package assessment provides Junior model evaluation through coding challenges.
package assessment

// Challenge represents a coding challenge for Junior assessment.
type Challenge struct {
	ID          string   // Unique identifier
	Category    string   // "algorithms", "data_structures", "code_quality", "real_world", "problem_solving"
	Name        string   // Human-readable name
	Difficulty  string   // "easy", "medium", "hard"
	Prompt      string   // The challenge prompt sent to Junior
	TestCases   []string // Example inputs/outputs for validation hints
	MaxScore    int      // Maximum points (typically 4)
	TimeLimit   int      // Seconds allowed (0 = default 60s)
}

// ChallengeSet returns the standard set of assessment challenges.
// These are designed to test a range of coding abilities.
func ChallengeSet() []Challenge {
	return []Challenge{
		// === Basic Algorithms (3 challenges) ===
		{
			ID:         "fizzbuzz",
			Category:   "algorithms",
			Name:       "FizzBuzz",
			Difficulty: "easy",
			Prompt: `Write a Python function called fizzbuzz(n) that returns a list of strings from 1 to n where:
- Numbers divisible by 3 are replaced with "Fizz"
- Numbers divisible by 5 are replaced with "Buzz"
- Numbers divisible by both 3 and 5 are replaced with "FizzBuzz"
- Other numbers are converted to strings

Example: fizzbuzz(5) should return ["1", "2", "Fizz", "4", "Buzz"]

Provide only the function implementation, no explanation needed.`,
			TestCases: []string{
				"fizzbuzz(5) == ['1', '2', 'Fizz', '4', 'Buzz']",
				"fizzbuzz(15)[-1] == 'FizzBuzz'",
			},
			MaxScore: 4,
		},
		{
			ID:         "palindrome",
			Category:   "algorithms",
			Name:       "Palindrome Check",
			Difficulty: "easy",
			Prompt: `Write a Python function called is_palindrome(s) that returns True if the string is a palindrome (ignoring case and non-alphanumeric characters), False otherwise.

Examples:
- is_palindrome("A man, a plan, a canal: Panama") == True
- is_palindrome("race a car") == False
- is_palindrome("") == True

Provide only the function implementation.`,
			TestCases: []string{
				"is_palindrome('racecar') == True",
				"is_palindrome('hello') == False",
			},
			MaxScore: 4,
		},
		{
			ID:         "fibonacci",
			Category:   "algorithms",
			Name:       "Fibonacci Sequence",
			Difficulty: "easy",
			Prompt: `Write a Python function called fibonacci(n) that returns the nth Fibonacci number (0-indexed).

Examples:
- fibonacci(0) == 0
- fibonacci(1) == 1
- fibonacci(10) == 55

The function should be efficient enough to handle n up to 30.
Provide only the function implementation.`,
			TestCases: []string{
				"fibonacci(0) == 0",
				"fibonacci(10) == 55",
			},
			MaxScore: 4,
		},

		// === Data Structures (3 challenges) ===
		{
			ID:         "two_sum",
			Category:   "data_structures",
			Name:       "Two Sum",
			Difficulty: "medium",
			Prompt: `Write a Python function called two_sum(nums, target) that returns the indices of two numbers in the list that add up to target. Assume exactly one solution exists.

Example: two_sum([2, 7, 11, 15], 9) should return [0, 1] (because nums[0] + nums[1] == 9)

Use an efficient approach (not brute force nested loops).
Provide only the function implementation.`,
			TestCases: []string{
				"sorted(two_sum([2, 7, 11, 15], 9)) == [0, 1]",
				"sorted(two_sum([3, 2, 4], 6)) == [1, 2]",
			},
			MaxScore: 4,
		},
		{
			ID:         "group_anagrams",
			Category:   "data_structures",
			Name:       "Group Anagrams",
			Difficulty: "medium",
			Prompt: `Write a Python function called group_anagrams(strs) that groups anagrams together from a list of strings.

Example: group_anagrams(["eat","tea","tan","ate","nat","bat"])
Should return something like: [["eat","tea","ate"], ["tan","nat"], ["bat"]]
(order of groups and within groups doesn't matter)

Provide only the function implementation.`,
			TestCases: []string{
				"len(group_anagrams(['eat','tea','tan','ate','nat','bat'])) == 3",
			},
			MaxScore: 4,
		},
		{
			ID:         "merge_intervals",
			Category:   "data_structures",
			Name:       "Merge Intervals",
			Difficulty: "medium",
			Prompt: `Write a Python function called merge_intervals(intervals) that merges overlapping intervals.

Example: merge_intervals([[1,3],[2,6],[8,10],[15,18]]) should return [[1,6],[8,10],[15,18]]

Provide only the function implementation.`,
			TestCases: []string{
				"merge_intervals([[1,3],[2,6],[8,10],[15,18]]) == [[1,6],[8,10],[15,18]]",
			},
			MaxScore: 4,
		},

		// === Code Quality (3 challenges) ===
		{
			ID:         "docstrings",
			Category:   "code_quality",
			Name:       "Add Docstrings",
			Difficulty: "easy",
			Prompt: `Add proper Google-style docstrings to this Python function:

def calculate_statistics(numbers):
    if not numbers:
        return None
    total = sum(numbers)
    count = len(numbers)
    mean = total / count
    sorted_nums = sorted(numbers)
    mid = count // 2
    if count % 2 == 0:
        median = (sorted_nums[mid-1] + sorted_nums[mid]) / 2
    else:
        median = sorted_nums[mid]
    return {"mean": mean, "median": median, "sum": total, "count": count}

Provide the complete function with comprehensive docstrings including Args, Returns, and Examples sections.`,
			MaxScore: 4,
		},
		{
			ID:         "type_hints",
			Category:   "code_quality",
			Name:       "Add Type Hints",
			Difficulty: "easy",
			Prompt: `Add complete type hints to this Python function:

def process_user_data(users, filter_active, sort_by):
    result = []
    for user in users:
        if filter_active and not user.get("active", False):
            continue
        result.append({
            "id": user["id"],
            "name": user["name"],
            "email": user.get("email", "")
        })
    if sort_by and sort_by in ["id", "name", "email"]:
        result.sort(key=lambda x: x[sort_by])
    return result

Provide the complete function with type hints for all parameters and return type. Use typing module types as needed (List, Dict, Optional, etc.).`,
			MaxScore: 4,
		},
		{
			ID:         "error_handling",
			Category:   "code_quality",
			Name:       "Add Error Handling",
			Difficulty: "medium",
			Prompt: `Improve this Python function with proper error handling:

def read_config(filepath):
    import json
    with open(filepath) as f:
        config = json.load(f)
    return {
        "host": config["database"]["host"],
        "port": config["database"]["port"],
        "name": config["database"]["name"]
    }

Add appropriate try/except blocks, handle missing keys gracefully, validate the config, and raise informative exceptions. Provide the complete improved function.`,
			MaxScore: 4,
		},

		// === Real-World (3 challenges) ===
		{
			ID:         "parse_log",
			Category:   "real_world",
			Name:       "Parse Log File",
			Difficulty: "medium",
			Prompt: `Write a Python function called parse_log_line(line) that parses a log line in this format:
"2024-01-15 10:30:45 [INFO] User john_doe logged in from 192.168.1.100"

Return a dictionary with keys: timestamp, level, message
Example output: {"timestamp": "2024-01-15 10:30:45", "level": "INFO", "message": "User john_doe logged in from 192.168.1.100"}

Handle edge cases like malformed lines (return None for invalid lines).
Provide only the function implementation.`,
			TestCases: []string{
				"parse_log_line('2024-01-15 10:30:45 [INFO] Test')['level'] == 'INFO'",
			},
			MaxScore: 4,
		},
		{
			ID:         "http_retry",
			Category:   "real_world",
			Name:       "HTTP Request with Retry",
			Difficulty: "medium",
			Prompt: `Write a Python function called fetch_with_retry(url, max_retries=3, timeout=5) that:
- Makes an HTTP GET request to the given URL
- Retries up to max_retries times on failure
- Uses exponential backoff between retries (1s, 2s, 4s)
- Returns the response text on success
- Raises an exception after all retries fail

Use the requests library. Provide only the function implementation.`,
			MaxScore: 4,
		},
		{
			ID:         "csv_processor",
			Category:   "real_world",
			Name:       "CSV Data Processor",
			Difficulty: "medium",
			Prompt: `Write a Python function called process_csv(filepath) that:
- Reads a CSV file with headers
- Returns a list of dictionaries (one per row)
- Handles missing values by using None
- Strips whitespace from all string values
- Converts numeric strings to int or float as appropriate

Use the csv module. Provide only the function implementation.`,
			MaxScore: 4,
		},

		// === Tool Use (5 challenges) - Requires Junior to use file/command tools ===
		{
			ID:         "tool_file_analysis",
			Category:   "tool_use",
			Name:       "File Analysis Task",
			Difficulty: "medium",
			Prompt: `I need you to analyze a project structure. Please:

1. Use list_directory to explore the current workspace root (".")
2. Find any Python files (.py) or Go files (.go) in the workspace
3. Use read_file to examine at least one source file you find
4. Write a brief summary of what you found to a file called "analysis_report.txt"

Use your tools to complete this task. Report what you discover.`,
			MaxScore:  4,
			TimeLimit: 90,
		},
		{
			ID:         "tool_search_extract",
			Category:   "tool_use",
			Name:       "Search and Extract",
			Difficulty: "medium",
			Prompt: `I need you to search for and extract information:

1. Use search_files to find any files containing the word "func" or "def" (function definitions)
2. Pick one interesting function you find
3. Use read_file to get the full context of that function
4. Create a file called "function_summary.txt" that contains:
   - The file path where you found it
   - The function name
   - A one-sentence description of what it does

Use your tools to complete this task.`,
			MaxScore:  4,
			TimeLimit: 90,
		},
		{
			ID:         "tool_create_script",
			Category:   "tool_use",
			Name:       "Create and Test Script",
			Difficulty: "medium",
			Prompt: `Create a simple Python script and verify it works:

1. Use write_file to create a file called "hello_test.py" containing:
   - A function called greet(name) that returns "Hello, {name}!"
   - A main block that prints greet("World")
2. Use execute_command to run: python3 hello_test.py
3. Report whether the script ran successfully and what it output

Use your tools to complete this task.`,
			MaxScore:  4,
			TimeLimit: 90,
		},
		{
			ID:         "tool_investigate_error",
			Category:   "tool_use",
			Name:       "Investigate Error",
			Difficulty: "hard",
			Prompt: `I have a buggy script. Please investigate and fix it:

First, create a file called "buggy_calc.py" with this content:
` + "```python" + `
def calculate_average(numbers):
    total = 0
    for n in numbers:
        total += n
    return total / len(numbers)

if __name__ == "__main__":
    result = calculate_average([])
    print(f"Average: {result}")
` + "```" + `

1. Use write_file to create this buggy script
2. Use execute_command to run it and observe the error
3. Use read_file to examine the code
4. Use write_file to create a fixed version called "fixed_calc.py" that handles the empty list case
5. Use execute_command to verify your fix works

Report what the bug was and how you fixed it.`,
			MaxScore:  4,
			TimeLimit: 120,
		},
		{
			ID:         "tool_multi_step",
			Category:   "tool_use",
			Name:       "Multi-Step Workflow",
			Difficulty: "hard",
			Prompt: `Complete this multi-step workflow using your tools:

1. Create a directory listing by using execute_command with "ls -la"
2. Save the output to a file called "dir_listing.txt" using write_file
3. Use search_files to find any README or documentation files
4. Create a final report file called "workspace_report.txt" that includes:
   - Number of files/directories in the workspace (from your ls output)
   - Names of any documentation files you found
   - A brief assessment of the workspace organization

This tests your ability to chain multiple tool operations together.`,
			MaxScore:  4,
			TimeLimit: 120,
		},

		// === Problem Solving (3 challenges) ===
		{
			ID:         "find_bug",
			Category:   "problem_solving",
			Name:       "Find the Bug",
			Difficulty: "medium",
			Prompt: `This function is supposed to find the longest substring without repeating characters, but it has bugs. Find and fix them:

def longest_unique_substring(s):
    if not s:
        return 0
    seen = {}
    start = 0
    max_length = 0
    for i, char in enumerate(s):
        if char in seen:
            start = seen[char] + 1
        seen[char] = i
        max_length = max(max_length, i - start + 1)
    return max_length

Test case that fails: longest_unique_substring("abba") should return 2, not 1.

Provide the corrected function and briefly explain the bug.`,
			MaxScore: 4,
		},
		{
			ID:         "optimize",
			Category:   "problem_solving",
			Name:       "Optimize Code",
			Difficulty: "hard",
			Prompt: `This function works but is inefficient. Optimize it:

def find_duplicates(nums):
    duplicates = []
    for i in range(len(nums)):
        for j in range(i + 1, len(nums)):
            if nums[i] == nums[j] and nums[i] not in duplicates:
                duplicates.append(nums[i])
    return duplicates

The current implementation is O(n²). Provide an O(n) solution.
Provide only the optimized function.`,
			MaxScore: 4,
		},
		{
			ID:         "refactor",
			Category:   "problem_solving",
			Name:       "Refactor Code",
			Difficulty: "hard",
			Prompt: `Refactor this messy function to be clean, readable, and maintainable:

def proc(d,t):
    r=[]
    for i in d:
        if t=="a":
            if i.get("status")=="active" and i.get("score",0)>50:
                r.append({"id":i["id"],"n":i["name"],"s":i["score"]})
        elif t=="i":
            if i.get("status")=="inactive":
                r.append({"id":i["id"],"n":i["name"],"s":i.get("score",0)})
        elif t=="h":
            if i.get("score",0)>90:
                r.append({"id":i["id"],"n":i["name"],"s":i["score"]})
    return sorted(r,key=lambda x:x["s"],reverse=True)

Improve variable names, add type hints, break into smaller functions if appropriate, and add a docstring. Provide the complete refactored code.`,
			MaxScore: 4,
		},
	}
}

// ChallengesByCategory groups challenges by their category.
func ChallengesByCategory() map[string][]Challenge {
	result := make(map[string][]Challenge)
	for _, c := range ChallengeSet() {
		result[c.Category] = append(result[c.Category], c)
	}
	return result
}

// CategoryOrder returns the display order for categories.
func CategoryOrder() []string {
	return []string{
		"algorithms",
		"data_structures",
		"code_quality",
		"real_world",
		"tool_use",
		"problem_solving",
	}
}

// CategoryNames maps category IDs to display names.
func CategoryNames() map[string]string {
	return map[string]string{
		"algorithms":      "Basic Algorithms",
		"data_structures": "Data Structures",
		"code_quality":    "Code Quality",
		"real_world":      "Real-World Tasks",
		"tool_use":        "Tool Use",
		"problem_solving": "Problem Solving",
	}
}
