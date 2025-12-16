// Package assessment provides Junior model evaluation through coding challenges.
package assessment

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// ChallengeGenerator creates variations of challenges for training data collection.
type ChallengeGenerator struct {
	rng *rand.Rand
}

// NewChallengeGenerator creates a new generator with a random seed.
func NewChallengeGenerator() *ChallengeGenerator {
	return &ChallengeGenerator{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GenerateExtendedSet generates n challenges with variations for training data collection.
func (g *ChallengeGenerator) GenerateExtendedSet(n int) []Challenge {
	challenges := make([]Challenge, 0, n)

	// Get base templates
	templates := g.getTemplates()

	// Generate n challenges by cycling through templates with variations
	for i := 0; i < n; i++ {
		template := templates[i%len(templates)]
		challenge := g.generateVariation(template, i)
		challenge.ID = fmt.Sprintf("%s_var_%d", template.ID, i)
		challenges = append(challenges, challenge)
	}

	return challenges
}

// Template holds a challenge template with variation parameters.
type Template struct {
	ID         string
	Category   string
	Name       string
	Difficulty string
	MaxScore   int
	TimeLimit  int
	Generator  func(idx int) string
}

func (g *ChallengeGenerator) getTemplates() []Template {
	var templates []Template
	templates = append(templates, g.algorithmTemplates()...)
	templates = append(templates, g.dataStructureTemplates()...)
	templates = append(templates, g.codeQualityTemplates()...)
	templates = append(templates, g.realWorldTemplates()...)
	templates = append(templates, g.toolUseTemplates()...)
	templates = append(templates, g.problemSolvingTemplates()...)
	return templates
}

func (g *ChallengeGenerator) generateVariation(t Template, idx int) Challenge {
	return Challenge{
		ID:         t.ID,
		Category:   t.Category,
		Name:       t.Name,
		Difficulty: t.Difficulty,
		MaxScore:   t.MaxScore,
		TimeLimit:  t.TimeLimit,
		Prompt:     t.Generator(idx),
	}
}

// === ALGORITHM TEMPLATES ===

func (g *ChallengeGenerator) algorithmTemplates() []Template {
	return []Template{
		{
			ID: "fizzbuzz", Category: "algorithms", Name: "FizzBuzz Variant",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genFizzBuzz,
		},
		{
			ID: "palindrome", Category: "algorithms", Name: "Palindrome Variant",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genPalindrome,
		},
		{
			ID: "fibonacci", Category: "algorithms", Name: "Fibonacci Variant",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genFibonacci,
		},
		{
			ID: "factorial", Category: "algorithms", Name: "Factorial",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genFactorial,
		},
		{
			ID: "prime_check", Category: "algorithms", Name: "Prime Number Check",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genPrimeCheck,
		},
		{
			ID: "binary_search", Category: "algorithms", Name: "Binary Search",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genBinarySearch,
		},
		{
			ID: "reverse_string", Category: "algorithms", Name: "Reverse String",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genReverseString,
		},
		{
			ID: "count_vowels", Category: "algorithms", Name: "Count Vowels",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genCountVowels,
		},
		{
			ID: "sum_digits", Category: "algorithms", Name: "Sum of Digits",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genSumDigits,
		},
		{
			ID: "gcd", Category: "algorithms", Name: "Greatest Common Divisor",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genGCD,
		},
	}
}

func (g *ChallengeGenerator) genFizzBuzz(idx int) string {
	// Vary the divisors and words
	divisors := []struct{ a, b int; wordA, wordB string }{
		{3, 5, "Fizz", "Buzz"},
		{2, 7, "Ping", "Pong"},
		{4, 6, "Foo", "Bar"},
		{3, 7, "Ding", "Dong"},
		{5, 11, "Hip", "Hop"},
	}
	v := divisors[idx%len(divisors)]
	n := 10 + g.rng.Intn(20)

	return fmt.Sprintf(`Write a Python function called fizzbuzz(n) that returns a list of strings from 1 to n where:
- Numbers divisible by %d are replaced with "%s"
- Numbers divisible by %d are replaced with "%s"
- Numbers divisible by both %d and %d are replaced with "%s%s"
- Other numbers are converted to strings

Example: fizzbuzz(%d) should handle numbers 1 to %d with these rules.

Provide only the function implementation, no explanation needed.`, v.a, v.wordA, v.b, v.wordB, v.a, v.b, v.wordA, v.wordB, n, n)
}

func (g *ChallengeGenerator) genPalindrome(idx int) string {
	funcNames := []string{"is_palindrome", "check_palindrome", "palindrome_check", "verify_palindrome"}
	examples := [][]string{
		{"A man, a plan, a canal: Panama", "race a car"},
		{"Was it a car or a cat I saw?", "hello world"},
		{"No lemon, no melon", "python code"},
		{"Eva, can I see bees in a cave?", "not a palindrome"},
	}
	fn := funcNames[idx%len(funcNames)]
	ex := examples[idx%len(examples)]

	return fmt.Sprintf(`Write a Python function called %s(s) that returns True if the string is a palindrome (ignoring case and non-alphanumeric characters), False otherwise.

Examples:
- %s("%s") == True
- %s("%s") == False
- %s("") == True

Provide only the function implementation.`, fn, fn, ex[0], fn, ex[1], fn)
}

func (g *ChallengeGenerator) genFibonacci(idx int) string {
	funcNames := []string{"fibonacci", "fib", "get_fibonacci", "fibonacci_number"}
	testN := []int{10, 15, 20, 12, 8}
	fn := funcNames[idx%len(funcNames)]
	n := testN[idx%len(testN)]
	// Precomputed fib values
	fibs := map[int]int{8: 21, 10: 55, 12: 144, 15: 610, 20: 6765}

	return fmt.Sprintf(`Write a Python function called %s(n) that returns the nth Fibonacci number (0-indexed).

Examples:
- %s(0) == 0
- %s(1) == 1
- %s(%d) == %d

The function should be efficient enough to handle n up to 30.
Provide only the function implementation.`, fn, fn, fn, fn, n, fibs[n])
}

func (g *ChallengeGenerator) genFactorial(idx int) string {
	funcNames := []string{"factorial", "fact", "calculate_factorial", "get_factorial"}
	fn := funcNames[idx%len(funcNames)]
	n := 5 + g.rng.Intn(5)
	// Calculate factorial
	result := 1
	for i := 2; i <= n; i++ {
		result *= i
	}

	return fmt.Sprintf(`Write a Python function called %s(n) that returns the factorial of n.

Examples:
- %s(0) == 1
- %s(1) == 1
- %s(%d) == %d

Handle negative inputs by raising a ValueError.
Provide only the function implementation.`, fn, fn, fn, fn, n, result)
}

func (g *ChallengeGenerator) genPrimeCheck(idx int) string {
	funcNames := []string{"is_prime", "check_prime", "prime_check", "is_prime_number"}
	fn := funcNames[idx%len(funcNames)]
	primes := []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 31}
	nonPrimes := []int{4, 6, 8, 9, 10, 12, 14, 15}
	p := primes[idx%len(primes)]
	np := nonPrimes[idx%len(nonPrimes)]

	return fmt.Sprintf(`Write a Python function called %s(n) that returns True if n is a prime number, False otherwise.

Examples:
- %s(%d) == True
- %s(%d) == False
- %s(1) == False
- %s(2) == True

Provide only the function implementation.`, fn, fn, p, fn, np, fn, fn)
}

func (g *ChallengeGenerator) genBinarySearch(idx int) string {
	funcNames := []string{"binary_search", "bin_search", "search_binary", "find_index"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(arr, target) that returns the index of target in a sorted array, or -1 if not found.

Examples:
- %s([1, 2, 3, 4, 5], 3) == 2
- %s([1, 2, 3, 4, 5], 6) == -1
- %s([], 1) == -1

Use the binary search algorithm (O(log n)), not linear search.
Provide only the function implementation.`, fn, fn, fn, fn)
}

func (g *ChallengeGenerator) genReverseString(idx int) string {
	funcNames := []string{"reverse_string", "reverse_str", "str_reverse", "flip_string"}
	fn := funcNames[idx%len(funcNames)]
	examples := []string{"hello", "Python", "OpenAI", "coding"}
	ex := examples[idx%len(examples)]
	reversed := reverseStr(ex)

	return fmt.Sprintf(`Write a Python function called %s(s) that returns the reversed string.

Examples:
- %s("%s") == "%s"
- %s("") == ""
- %s("a") == "a"

Provide only the function implementation.`, fn, fn, ex, reversed, fn, fn)
}

func reverseStr(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func (g *ChallengeGenerator) genCountVowels(idx int) string {
	funcNames := []string{"count_vowels", "vowel_count", "num_vowels", "get_vowel_count"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(s) that returns the number of vowels (a, e, i, o, u) in a string, case-insensitive.

Examples:
- %s("hello") == 2
- %s("AEIOU") == 5
- %s("rhythm") == 0
- %s("") == 0

Provide only the function implementation.`, fn, fn, fn, fn, fn)
}

func (g *ChallengeGenerator) genSumDigits(idx int) string {
	funcNames := []string{"sum_digits", "digit_sum", "add_digits", "sum_of_digits"}
	fn := funcNames[idx%len(funcNames)]
	nums := []struct{ n, sum int }{{123, 6}, {9999, 36}, {100, 1}, {12345, 15}}
	v := nums[idx%len(nums)]

	return fmt.Sprintf(`Write a Python function called %s(n) that returns the sum of all digits in a non-negative integer.

Examples:
- %s(%d) == %d
- %s(0) == 0
- %s(5) == 5

Provide only the function implementation.`, fn, fn, v.n, v.sum, fn, fn)
}

func (g *ChallengeGenerator) genGCD(idx int) string {
	funcNames := []string{"gcd", "greatest_common_divisor", "find_gcd", "compute_gcd"}
	fn := funcNames[idx%len(funcNames)]
	pairs := []struct{ a, b, gcd int }{{48, 18, 6}, {100, 25, 25}, {17, 13, 1}, {54, 24, 6}}
	v := pairs[idx%len(pairs)]

	return fmt.Sprintf(`Write a Python function called %s(a, b) that returns the greatest common divisor of two positive integers.

Examples:
- %s(%d, %d) == %d
- %s(1, 1) == 1
- %s(10, 5) == 5

Use the Euclidean algorithm.
Provide only the function implementation.`, fn, fn, v.a, v.b, v.gcd, fn, fn)
}

// === DATA STRUCTURE TEMPLATES ===

func (g *ChallengeGenerator) dataStructureTemplates() []Template {
	return []Template{
		{
			ID: "two_sum", Category: "data_structures", Name: "Two Sum Variant",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genTwoSum,
		},
		{
			ID: "reverse_list", Category: "data_structures", Name: "Reverse Linked List",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genReverseList,
		},
		{
			ID: "valid_parentheses", Category: "data_structures", Name: "Valid Parentheses",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genValidParentheses,
		},
		{
			ID: "merge_sorted", Category: "data_structures", Name: "Merge Sorted Arrays",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genMergeSorted,
		},
		{
			ID: "remove_duplicates", Category: "data_structures", Name: "Remove Duplicates",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genRemoveDuplicates,
		},
		{
			ID: "max_subarray", Category: "data_structures", Name: "Maximum Subarray",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genMaxSubarray,
		},
		{
			ID: "rotate_array", Category: "data_structures", Name: "Rotate Array",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genRotateArray,
		},
		{
			ID: "intersection", Category: "data_structures", Name: "Array Intersection",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genIntersection,
		},
	}
}

func (g *ChallengeGenerator) genTwoSum(idx int) string {
	funcNames := []string{"two_sum", "find_two_sum", "sum_indices", "find_pair"}
	fn := funcNames[idx%len(funcNames)]
	examples := []struct{ nums string; target int; result string }{
		{"[2, 7, 11, 15]", 9, "[0, 1]"},
		{"[3, 2, 4]", 6, "[1, 2]"},
		{"[1, 5, 3, 7]", 8, "[1, 2]"},
	}
	ex := examples[idx%len(examples)]

	return fmt.Sprintf(`Write a Python function called %s(nums, target) that returns the indices of two numbers in the list that add up to target. Assume exactly one solution exists.

Example: %s(%s, %d) should return %s

Use an efficient approach (O(n) with hash map, not O(n²) brute force).
Provide only the function implementation.`, fn, fn, ex.nums, ex.target, ex.result)
}

func (g *ChallengeGenerator) genReverseList(idx int) string {
	return `Write a Python function called reverse_linked_list(head) that reverses a singly linked list in-place.

Assume the ListNode class is defined as:
class ListNode:
    def __init__(self, val=0, next=None):
        self.val = val
        self.next = next

The function should return the new head of the reversed list.
Provide only the function implementation.`
}

func (g *ChallengeGenerator) genValidParentheses(idx int) string {
	funcNames := []string{"is_valid", "valid_parentheses", "check_brackets", "balanced_brackets"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(s) that returns True if the string has valid/balanced parentheses, brackets, and braces.

Examples:
- %s("()") == True
- %s("()[]{}") == True
- %s("(]") == False
- %s("([)]") == False
- %s("{[]}") == True

Provide only the function implementation.`, fn, fn, fn, fn, fn, fn)
}

func (g *ChallengeGenerator) genMergeSorted(idx int) string {
	funcNames := []string{"merge_sorted", "merge_arrays", "merge_lists", "combine_sorted"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(arr1, arr2) that merges two sorted arrays into one sorted array.

Examples:
- %s([1, 3, 5], [2, 4, 6]) == [1, 2, 3, 4, 5, 6]
- %s([], [1, 2]) == [1, 2]
- %s([1], []) == [1]

Do this in O(n+m) time where n and m are the array lengths.
Provide only the function implementation.`, fn, fn, fn, fn)
}

func (g *ChallengeGenerator) genRemoveDuplicates(idx int) string {
	funcNames := []string{"remove_duplicates", "unique_elements", "deduplicate", "remove_dups"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(nums) that removes duplicates from a sorted array in-place and returns the new length.

Examples:
- %s([1, 1, 2]) should modify array to [1, 2, ...] and return 2
- %s([0, 0, 1, 1, 1, 2, 2, 3, 3, 4]) should return 5

Provide only the function implementation.`, fn, fn, fn)
}

func (g *ChallengeGenerator) genMaxSubarray(idx int) string {
	funcNames := []string{"max_subarray", "max_sum_subarray", "kadane", "largest_sum"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(nums) that finds the contiguous subarray with the largest sum and returns that sum.

Examples:
- %s([-2, 1, -3, 4, -1, 2, 1, -5, 4]) == 6 (subarray [4, -1, 2, 1])
- %s([1]) == 1
- %s([5, 4, -1, 7, 8]) == 23

Use Kadane's algorithm for O(n) time complexity.
Provide only the function implementation.`, fn, fn, fn, fn)
}

func (g *ChallengeGenerator) genRotateArray(idx int) string {
	funcNames := []string{"rotate_array", "rotate", "rotate_right", "shift_array"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(nums, k) that rotates the array to the right by k steps in-place.

Examples:
- %s([1, 2, 3, 4, 5], 2) modifies array to [4, 5, 1, 2, 3]
- %s([1, 2], 3) modifies array to [2, 1]

Handle k larger than array length.
Provide only the function implementation.`, fn, fn, fn)
}

func (g *ChallengeGenerator) genIntersection(idx int) string {
	funcNames := []string{"intersection", "array_intersection", "common_elements", "find_common"}
	fn := funcNames[idx%len(funcNames)]

	return fmt.Sprintf(`Write a Python function called %s(nums1, nums2) that returns the intersection of two arrays (unique elements that appear in both).

Examples:
- %s([1, 2, 2, 1], [2, 2]) == [2]
- %s([4, 9, 5], [9, 4, 9, 8, 4]) == [4, 9] (order doesn't matter)

Provide only the function implementation.`, fn, fn, fn)
}

// === CODE QUALITY TEMPLATES ===

func (g *ChallengeGenerator) codeQualityTemplates() []Template {
	return []Template{
		{
			ID: "docstrings", Category: "code_quality", Name: "Add Docstrings",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genDocstrings,
		},
		{
			ID: "type_hints", Category: "code_quality", Name: "Add Type Hints",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genTypeHints,
		},
		{
			ID: "error_handling", Category: "code_quality", Name: "Add Error Handling",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genErrorHandling,
		},
		{
			ID: "rename_variables", Category: "code_quality", Name: "Improve Variable Names",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genRenameVariables,
		},
	}
}

func (g *ChallengeGenerator) genDocstrings(idx int) string {
	functions := []string{
		`def calculate_discount(price, discount_percent):
    if discount_percent < 0 or discount_percent > 100:
        return price
    discount = price * (discount_percent / 100)
    return price - discount`,
		`def find_average(numbers):
    if not numbers:
        return 0
    return sum(numbers) / len(numbers)`,
		`def validate_email(email):
    import re
    pattern = r'^[\w\.-]+@[\w\.-]+\.\w+$'
    return bool(re.match(pattern, email))`,
		`def merge_dicts(dict1, dict2):
    result = dict1.copy()
    result.update(dict2)
    return result`,
	}
	fn := functions[idx%len(functions)]

	return fmt.Sprintf(`Add proper Google-style docstrings to this Python function:

%s

Provide the complete function with comprehensive docstrings including Args, Returns, and Examples sections.`, fn)
}

func (g *ChallengeGenerator) genTypeHints(idx int) string {
	functions := []string{
		`def filter_by_age(users, min_age, max_age):
    return [u for u in users if min_age <= u.get("age", 0) <= max_age]`,
		`def format_name(first, last, middle=None):
    if middle:
        return f"{first} {middle} {last}"
    return f"{first} {last}"`,
		`def parse_config(config_str, defaults):
    import json
    parsed = json.loads(config_str)
    result = defaults.copy()
    result.update(parsed)
    return result`,
	}
	fn := functions[idx%len(functions)]

	return fmt.Sprintf(`Add complete type hints to this Python function:

%s

Provide the complete function with type hints for all parameters and return type. Use typing module types as needed (List, Dict, Optional, etc.).`, fn)
}

func (g *ChallengeGenerator) genErrorHandling(idx int) string {
	functions := []string{
		`def divide_numbers(a, b):
    return a / b`,
		`def get_user_by_id(users, user_id):
    for user in users:
        if user["id"] == user_id:
            return user
    return users[0]  # fallback`,
		`def read_json_file(filepath):
    import json
    with open(filepath) as f:
        return json.load(f)`,
	}
	fn := functions[idx%len(functions)]

	return fmt.Sprintf(`Improve this Python function with proper error handling:

%s

Add appropriate try/except blocks, handle edge cases gracefully, validate inputs, and raise informative exceptions. Provide the complete improved function.`, fn)
}

func (g *ChallengeGenerator) genRenameVariables(idx int) string {
	functions := []string{
		`def p(d, k):
    r = []
    for i in d:
        if k in i:
            r.append(i[k])
    return r`,
		`def c(l, v):
    n = 0
    for x in l:
        if x == v:
            n += 1
    return n`,
		`def f(s, c):
    r = ""
    for x in s:
        if x != c:
            r += x
    return r`,
	}
	fn := functions[idx%len(functions)]

	return fmt.Sprintf(`Refactor this function with better variable names and add a docstring:

%s

Make the code self-documenting with clear, descriptive names. Provide the complete improved function.`, fn)
}

// === REAL WORLD TEMPLATES ===

func (g *ChallengeGenerator) realWorldTemplates() []Template {
	return []Template{
		{
			ID: "parse_log", Category: "real_world", Name: "Parse Log Line",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genParseLog,
		},
		{
			ID: "http_retry", Category: "real_world", Name: "HTTP with Retry",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genHTTPRetry,
		},
		{
			ID: "csv_processor", Category: "real_world", Name: "CSV Processor",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genCSVProcessor,
		},
		{
			ID: "cache", Category: "real_world", Name: "Simple Cache",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genCache,
		},
		{
			ID: "rate_limiter", Category: "real_world", Name: "Rate Limiter",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genRateLimiter,
		},
		{
			ID: "password_validator", Category: "real_world", Name: "Password Validator",
			Difficulty: "easy", MaxScore: 4,
			Generator: g.genPasswordValidator,
		},
	}
}

func (g *ChallengeGenerator) genParseLog(idx int) string {
	formats := []struct{ format, example string }{
		{
			"YYYY-MM-DD HH:MM:SS [LEVEL] Message",
			"2024-01-15 10:30:45 [INFO] User logged in",
		},
		{
			"[LEVEL] YYYY/MM/DD HH:MM:SS - Message",
			"[ERROR] 2024/01/15 10:30:45 - Connection failed",
		},
		{
			"LEVEL: Message (timestamp: YYYY-MM-DD HH:MM:SS)",
			"WARNING: Disk space low (timestamp: 2024-01-15 10:30:45)",
		},
	}
	f := formats[idx%len(formats)]

	return fmt.Sprintf(`Write a Python function called parse_log_line(line) that parses a log line in this format:
"%s"

Example input: "%s"
Return a dictionary with keys: timestamp, level, message

Handle edge cases like malformed lines (return None for invalid lines).
Provide only the function implementation.`, f.format, f.example)
}

func (g *ChallengeGenerator) genHTTPRetry(idx int) string {
	retries := []int{3, 5, 2}
	backoffs := []string{"1s, 2s, 4s", "1s, 3s, 9s", "2s, 4s"}
	r := retries[idx%len(retries)]
	b := backoffs[idx%len(backoffs)]

	return fmt.Sprintf(`Write a Python function called fetch_with_retry(url, max_retries=%d, timeout=5) that:
- Makes an HTTP GET request to the given URL
- Retries up to max_retries times on failure
- Uses exponential backoff between retries (%s)
- Returns the response text on success
- Raises an exception after all retries fail

Use the requests library. Provide only the function implementation.`, r, b)
}

func (g *ChallengeGenerator) genCSVProcessor(idx int) string {
	return `Write a Python function called process_csv(filepath) that:
- Reads a CSV file with headers
- Returns a list of dictionaries (one per row)
- Handles missing values by using None
- Strips whitespace from all string values
- Converts numeric strings to int or float as appropriate

Use the csv module. Provide only the function implementation.`
}

func (g *ChallengeGenerator) genCache(idx int) string {
	return `Write a Python class called SimpleCache that implements a basic cache with expiration:

- __init__(self, default_ttl=60): Initialize with default TTL in seconds
- set(self, key, value, ttl=None): Store a value with optional custom TTL
- get(self, key): Return value if exists and not expired, else None
- delete(self, key): Remove a key from cache
- clear(self): Remove all entries

Provide the complete class implementation.`
}

func (g *ChallengeGenerator) genRateLimiter(idx int) string {
	limits := []struct{ requests, seconds int }{
		{10, 60},
		{100, 3600},
		{5, 1},
	}
	l := limits[idx%len(limits)]

	return fmt.Sprintf(`Write a Python class called RateLimiter that limits requests per time window:

- __init__(self, max_requests=%d, window_seconds=%d)
- allow(self) -> bool: Returns True if request is allowed, False if rate limited
- reset(self): Resets the limiter

Use a sliding window approach.
Provide the complete class implementation.`, l.requests, l.seconds)
}

func (g *ChallengeGenerator) genPasswordValidator(idx int) string {
	rules := []string{
		"at least 8 characters, one uppercase, one lowercase, one digit",
		"at least 10 characters, one uppercase, one lowercase, one digit, one special character",
		"at least 6 characters, must contain a digit and a letter",
	}
	r := rules[idx%len(rules)]

	return fmt.Sprintf(`Write a Python function called validate_password(password) that returns True if the password meets these requirements:
- %s

Return False if any requirement is not met.
Provide only the function implementation.`, r)
}

// === TOOL USE TEMPLATES ===

func (g *ChallengeGenerator) toolUseTemplates() []Template {
	return []Template{
		{
			ID: "tool_list_and_read", Category: "tool_use", Name: "List and Read Files",
			Difficulty: "medium", MaxScore: 4, TimeLimit: 120,
			Generator: g.genToolListRead,
		},
		{
			ID: "tool_search_pattern", Category: "tool_use", Name: "Search for Pattern",
			Difficulty: "medium", MaxScore: 4, TimeLimit: 120,
			Generator: g.genToolSearchPattern,
		},
		{
			ID: "tool_create_file", Category: "tool_use", Name: "Create and Verify File",
			Difficulty: "medium", MaxScore: 4, TimeLimit: 120,
			Generator: g.genToolCreateFile,
		},
		{
			ID: "tool_execute_script", Category: "tool_use", Name: "Execute and Report",
			Difficulty: "medium", MaxScore: 4, TimeLimit: 120,
			Generator: g.genToolExecuteScript,
		},
		{
			ID: "tool_multi_step", Category: "tool_use", Name: "Multi-Step Workflow",
			Difficulty: "hard", MaxScore: 4, TimeLimit: 180,
			Generator: g.genToolMultiStep,
		},
		{
			ID: "tool_investigate", Category: "tool_use", Name: "Investigate and Fix",
			Difficulty: "hard", MaxScore: 4, TimeLimit: 180,
			Generator: g.genToolInvestigate,
		},
		{
			ID: "tool_analyze_code", Category: "tool_use", Name: "Analyze Codebase",
			Difficulty: "medium", MaxScore: 4, TimeLimit: 120,
			Generator: g.genToolAnalyzeCode,
		},
		{
			ID: "tool_create_test", Category: "tool_use", Name: "Create Test File",
			Difficulty: "medium", MaxScore: 4, TimeLimit: 120,
			Generator: g.genToolCreateTest,
		},
	}
}

func (g *ChallengeGenerator) genToolListRead(idx int) string {
	return `Use your tools to explore the workspace:

1. Use list_directory to list the contents of the current directory (".")
2. Identify any source code files (Python, Go, etc.)
3. Use read_file to examine at least one source file
4. Use context_write to record your findings
5. Summarize what you found

Remember: INVOKE the tools, don't just describe what you would do.`
}

func (g *ChallengeGenerator) genToolSearchPattern(idx int) string {
	patterns := []string{"func", "def", "class", "import", "return"}
	p := patterns[idx%len(patterns)]

	return fmt.Sprintf(`Use your tools to search for code patterns:

1. Use search_files to find files containing "%s"
2. Pick one interesting match
3. Use read_file to examine that file
4. Use write_file to save your analysis to "search_results.txt"
5. Report what you discovered

Remember: INVOKE the tools, don't just describe what you would do.`, p)
}

func (g *ChallengeGenerator) genToolCreateFile(idx int) string {
	filenames := []string{"greeting.py", "math_utils.py", "string_helper.py", "data_processor.py"}
	fn := filenames[idx%len(filenames)]

	return fmt.Sprintf(`Create a Python utility file:

1. Use write_file to create "%s" with at least two useful functions
2. Use read_file to verify the file was created correctly
3. Use execute_command to run "python3 -c 'import %s; print(dir(%s))'" to verify it's valid Python
4. Report the result

Remember: INVOKE the tools, don't just describe what you would do.`, fn, strings.TrimSuffix(fn, ".py"), strings.TrimSuffix(fn, ".py"))
}

func (g *ChallengeGenerator) genToolExecuteScript(idx int) string {
	return `Create and run a test script:

1. Use write_file to create "test_script.py" that:
   - Defines a simple function
   - Has a main block that tests the function
   - Prints "TEST PASSED" if successful
2. Use execute_command to run: python3 test_script.py
3. Report whether the test passed

Remember: INVOKE the tools, don't just describe what you would do.`
}

func (g *ChallengeGenerator) genToolMultiStep(idx int) string {
	return `Complete this multi-step workflow:

1. Use execute_command to run "ls -la" and capture the output
2. Use write_file to save that output to "directory_info.txt"
3. Use search_files to find any documentation files (README, .md files)
4. Use write_file to create "workspace_summary.txt" with:
   - Count of files in the directory
   - List of documentation files found
   - Brief assessment of the workspace

Track your progress with context_write.
Remember: INVOKE the tools, don't just describe what you would do.`
}

func (g *ChallengeGenerator) genToolInvestigate(idx int) string {
	bugs := []struct{ code, bug string }{
		{`def divide(a, b):
    return a / b

print(divide(10, 0))`, "division by zero"},
		{`def get_first(items):
    return items[0]

print(get_first([]))`, "index out of range"},
		{`def parse_int(s):
    return int(s)

print(parse_int("hello"))`, "invalid literal"},
	}
	b := bugs[idx%len(bugs)]

	return fmt.Sprintf(`Investigate and fix this buggy script:

1. Use write_file to create "buggy_script.py" with this code:
%s

2. Use execute_command to run it and observe the error
3. Use read_file to examine the code
4. Use write_file to create "fixed_script.py" with a corrected version
5. Use execute_command to verify the fix works
6. Report what the bug was (%s) and how you fixed it

Remember: INVOKE the tools, don't just describe what you would do.`, "```python\n"+b.code+"\n```", b.bug)
}

func (g *ChallengeGenerator) genToolAnalyzeCode(idx int) string {
	return `Analyze the codebase structure:

1. Use list_directory to explore "." (current directory)
2. Use search_files to find function definitions ("def " or "func ")
3. Count how many functions are defined in each file
4. Use write_file to create "code_analysis.txt" with your findings
5. Summarize the codebase structure

Remember: INVOKE the tools, don't just describe what you would do.`
}

func (g *ChallengeGenerator) genToolCreateTest(idx int) string {
	return `Create a test file for a simple module:

1. Use write_file to create "calculator.py" with functions: add, subtract, multiply, divide
2. Use write_file to create "test_calculator.py" with unit tests using unittest
3. Use execute_command to run: python3 -m pytest test_calculator.py -v (or python3 -m unittest test_calculator -v)
4. Report the test results

Remember: INVOKE the tools, don't just describe what you would do.`
}

// === PROBLEM SOLVING TEMPLATES ===

func (g *ChallengeGenerator) problemSolvingTemplates() []Template {
	return []Template{
		{
			ID: "find_bug", Category: "problem_solving", Name: "Find and Fix Bug",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genFindBug,
		},
		{
			ID: "optimize", Category: "problem_solving", Name: "Optimize Code",
			Difficulty: "hard", MaxScore: 4,
			Generator: g.genOptimize,
		},
		{
			ID: "refactor", Category: "problem_solving", Name: "Refactor Code",
			Difficulty: "hard", MaxScore: 4,
			Generator: g.genRefactor,
		},
		{
			ID: "edge_cases", Category: "problem_solving", Name: "Handle Edge Cases",
			Difficulty: "medium", MaxScore: 4,
			Generator: g.genEdgeCases,
		},
	}
}

func (g *ChallengeGenerator) genFindBug(idx int) string {
	bugs := []struct{ code, testCase, expected string }{
		{`def longest_unique_substring(s):
    seen = {}
    start = 0
    max_length = 0
    for i, char in enumerate(s):
        if char in seen:
            start = seen[char] + 1
        seen[char] = i
        max_length = max(max_length, i - start + 1)
    return max_length`, `longest_unique_substring("abba")`, "2"},
		{`def binary_search(arr, target):
    left, right = 0, len(arr)
    while left < right:
        mid = (left + right) // 2
        if arr[mid] == target:
            return mid
        elif arr[mid] < target:
            left = mid
        else:
            right = mid
    return -1`, `binary_search([1,2,3,4,5], 3)`, "2 (may infinite loop)"},
		{`def merge_sort(arr):
    if len(arr) <= 1:
        return arr
    mid = len(arr) // 2
    left = merge_sort(arr[:mid])
    right = merge_sort(arr[mid:])
    return merge(left, right)

def merge(left, right):
    result = []
    i = j = 0
    while i < len(left) and j < len(right):
        if left[i] < right[j]:
            result.append(left[i])
            i += 1
        else:
            result.append(right[j])
            j += 1
    return result`, `merge_sort([3,1,2])`, "[1,2,3] (missing remaining elements)"},
	}
	b := bugs[idx%len(bugs)]

	return fmt.Sprintf(`This function has a bug. Find and fix it:

%s

Test case that may fail: %s should return %s

Provide the corrected function and briefly explain the bug.`, "```python\n"+b.code+"\n```", b.testCase, b.expected)
}

func (g *ChallengeGenerator) genOptimize(idx int) string {
	inefficient := []struct{ code, complexity string }{
		{`def find_duplicates(nums):
    duplicates = []
    for i in range(len(nums)):
        for j in range(i + 1, len(nums)):
            if nums[i] == nums[j] and nums[i] not in duplicates:
                duplicates.append(nums[i])
    return duplicates`, "O(n²) to O(n)"},
		{`def has_pair_with_sum(nums, target):
    for i in range(len(nums)):
        for j in range(i + 1, len(nums)):
            if nums[i] + nums[j] == target:
                return True
    return False`, "O(n²) to O(n)"},
		{`def count_occurrences(words):
    result = {}
    for word in words:
        count = 0
        for w in words:
            if w == word:
                count += 1
        result[word] = count
    return result`, "O(n²) to O(n)"},
	}
	b := inefficient[idx%len(inefficient)]

	return fmt.Sprintf(`This function works but is inefficient. Optimize it:

%s

The current implementation is %s. Provide an optimized solution.
Provide only the optimized function.`, "```python\n"+b.code+"\n```", b.complexity)
}

func (g *ChallengeGenerator) genRefactor(idx int) string {
	messyCode := []string{
		`def p(d,t):
    r=[]
    for i in d:
        if t=="a":
            if i.get("status")=="active":
                r.append({"id":i["id"],"n":i["name"]})
        elif t=="i":
            if i.get("status")=="inactive":
                r.append({"id":i["id"],"n":i["name"]})
    return r`,
		`def c(x,y,o):
    if o=="+":return x+y
    elif o=="-":return x-y
    elif o=="*":return x*y
    elif o=="/":return x/y if y!=0 else None
    else:return None`,
	}
	m := messyCode[idx%len(messyCode)]

	return fmt.Sprintf(`Refactor this messy function to be clean, readable, and maintainable:

%s

Improve variable names, add type hints, break into smaller functions if appropriate, and add a docstring. Provide the complete refactored code.`, "```python\n"+m+"\n```")
}

func (g *ChallengeGenerator) genEdgeCases(idx int) string {
	functions := []struct{ code, cases string }{
		{`def calculate_average(numbers):
    return sum(numbers) / len(numbers)`, "empty list, single element, negative numbers"},
		{`def find_max(items):
    max_val = items[0]
    for item in items:
        if item > max_val:
            max_val = item
    return max_val`, "empty list, single element, all same values, negative numbers"},
		{`def get_element(arr, index):
    return arr[index]`, "empty array, negative index, index out of bounds"},
	}
	f := functions[idx%len(functions)]

	return fmt.Sprintf(`This function doesn't handle edge cases. Add proper handling:

%s

Edge cases to handle: %s

Provide the complete function with edge case handling and appropriate error messages or default values.`, "```python\n"+f.code+"\n```", f.cases)
}
