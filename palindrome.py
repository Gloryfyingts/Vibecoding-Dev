def is_palindrome(text):
    cleaned = text.lower().replace(" ", "")
    return cleaned == cleaned[::-1]


if __name__ == "__main__":
    words = ["racecar", "hello", "madam", "python", "level"]
    for word in words:
        result = "YES" if is_palindrome(word) else "NO"
        print(f"{word} -> {result}")
