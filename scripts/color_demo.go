package main

import (
	"fmt"
	"nfl-app-go/logging"
)

func colorDemo() {
	fmt.Println("=== ANSI Color Reference ===")
	fmt.Println()

	// Current logging system colors
	fmt.Println("🔹 Current Logging System Colors:")
	fmt.Printf("\033[36mDEBUG\033[0m   - Cyan     (\\033[36m)\n")
	fmt.Printf("\033[32mINFO\033[0m    - Green    (\\033[32m)\n")
	fmt.Printf("\033[33mWARN\033[0m    - Yellow   (\\033[33m)\n")
	fmt.Printf("\033[31mERROR\033[0m   - Red      (\\033[31m)\n")
	fmt.Printf("\033[35mFATAL\033[0m   - Magenta  (\\033[35m)\n")
	fmt.Println()

	// Basic foreground colors
	fmt.Println("🎨 Standard Foreground Colors (\\033[3Xm):")
	colors := []struct {
		code int
		name string
	}{
		{30, "Black"},
		{31, "Red"},
		{32, "Green"},
		{33, "Yellow"},
		{34, "Blue"},
		{35, "Magenta"},
		{36, "Cyan"},
		{37, "White"},
	}

	for _, color := range colors {
		fmt.Printf("\033[%dm%s\033[0m - Code: \\033[%dm\n", color.code, color.name, color.code)
	}
	fmt.Println()

	// Bright foreground colors
	fmt.Println("✨ Bright Foreground Colors (\\033[9Xm):")
	brightColors := []struct {
		code int
		name string
	}{
		{90, "Bright Black (Gray)"},
		{91, "Bright Red"},
		{92, "Bright Green"},
		{93, "Bright Yellow"},
		{94, "Bright Blue"},
		{95, "Bright Magenta"},
		{96, "Bright Cyan"},
		{97, "Bright White"},
	}

	for _, color := range brightColors {
		fmt.Printf("\033[%dm%s\033[0m - Code: \\033[%dm\n", color.code, color.name, color.code)
	}
	fmt.Println()

	// Background colors
	fmt.Println("🌈 Background Colors (\\033[4Xm):")
	bgColors := []struct {
		code int
		name string
	}{
		{40, "Black Background"},
		{41, "Red Background"},
		{42, "Green Background"},
		{43, "Yellow Background"},
		{44, "Blue Background"},
		{45, "Magenta Background"},
		{46, "Cyan Background"},
		{47, "White Background"},
	}

	for _, color := range bgColors {
		fmt.Printf("\033[%dm%s\033[0m - Code: \\033[%dm\n", color.code, color.name, color.code)
	}
	fmt.Println()

	// Text formatting
	fmt.Println("📝 Text Formatting:")
	fmt.Printf("\033[1mBold\033[0m - Code: \\033[1m\n")
	fmt.Printf("\033[2mDim\033[0m - Code: \\033[2m\n")
	fmt.Printf("\033[3mItalic\033[0m - Code: \\033[3m\n")
	fmt.Printf("\033[4mUnderline\033[0m - Code: \\033[4m\n")
	fmt.Printf("\033[5mBlink\033[0m - Code: \\033[5m\n")
	fmt.Printf("\033[7mReverse\033[0m - Code: \\033[7m\n")
	fmt.Printf("\033[9mStrikethrough\033[0m - Code: \\033[9m\n")
	fmt.Println()

	// Combinations
	fmt.Println("🎭 Color Combinations:")
	fmt.Printf("\033[1;31mBold Red\033[0m - Code: \\033[1;31m\n")
	fmt.Printf("\033[1;32mBold Green\033[0m - Code: \\033[1;32m\n")
	fmt.Printf("\033[4;34mUnderlined Blue\033[0m - Code: \\033[4;34m\n")
	fmt.Printf("\033[1;4;35mBold Underlined Magenta\033[0m - Code: \\033[1;4;35m\n")
	fmt.Printf("\033[3;36mItalic Cyan\033[0m - Code: \\033[3;36m\n")
	fmt.Printf("\033[1;93mBold Bright Yellow\033[0m - Code: \\033[1;93m\n")
	fmt.Printf("\033[7;31mReverse Red\033[0m - Code: \\033[7;31m\n")
	fmt.Println()

	// 256-color palette comprehensive display
	fmt.Println("🎨 Extended 256-Color Palette (\\033[38;5;Nm):")

	// Standard 16 colors (0-15)
	fmt.Println("Standard Colors (0-15):")
	for i := 0; i <= 15; i++ {
		if i == 8 {
			fmt.Println() // New line after first 8
		}
		fmt.Printf("\033[38;5;%dm%3d\033[0m ", i, i)
	}
	fmt.Println()
	fmt.Println()

	// 216 color cube (16-231) - 6x6x6 RGB cube
	fmt.Println("RGB Color Cube (16-231) - 6×6×6 combinations:")
	for r := 0; r < 6; r++ {
		fmt.Printf("Red level %d: ", r)
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				color := 16 + (36 * r) + (6 * g) + b
				fmt.Printf("\033[38;5;%dm%3d\033[0m ", color, color)
			}
			fmt.Print(" ")
		}
		fmt.Println()
	}
	fmt.Println()

	// Grayscale colors (232-255) - 24 shades of gray
	fmt.Println("Grayscale Ramp (232-255) - 24 shades:")
	for i := 232; i <= 255; i++ {
		fmt.Printf("\033[38;5;%dm%3d\033[0m ", i, i)
		if (i-232+1)%12 == 0 {
			fmt.Println() // New line every 12 colors
		}
	}
	fmt.Println()

	// Popular color selections
	fmt.Println("Popular Extended Colors:")
	popularColors := map[int]string{
		196: "Bright Red",
		46:  "Bright Green",
		21:  "Bright Blue",
		226: "Bright Yellow",
		201: "Hot Pink",
		51:  "Cyan",
		208: "Orange",
		129: "Purple",
		118: "Light Green",
		39:  "Sky Blue",
		220: "Gold",
		160: "Dark Red",
		28:  "Forest Green",
		94:  "Dark Purple",
		172: "Orange Red",
		33:  "Deep Blue",
		165: "Magenta",
		214: "Orange Yellow",
		124: "Dark Magenta",
		88:  "Dark Orange",
	}

	for color, name := range popularColors {
		fmt.Printf("\033[38;5;%dm%3d %-15s\033[0m ", color, color, name)
		if (len(fmt.Sprintf("%3d %-15s", color, name)) * 2) % 80 > 60 {
			fmt.Println()
		}
	}
	fmt.Println()
	fmt.Println()

	// RGB colors (24-bit)
	fmt.Println("🌟 RGB Colors (\\033[38;2;R;G;Bm):")
	fmt.Printf("\033[38;2;255;0;0mBright Red RGB\033[0m - Code: \\033[38;2;255;0;0m\n")
	fmt.Printf("\033[38;2;0;255;0mBright Green RGB\033[0m - Code: \\033[38;2;0;255;0m\n")
	fmt.Printf("\033[38;2;0;0;255mBright Blue RGB\033[0m - Code: \\033[38;2;0;0;255m\n")
	fmt.Printf("\033[38;2;255;165;0mOrange RGB\033[0m - Code: \\033[38;2;255;165;0m\n")
	fmt.Printf("\033[38;2;128;0;128mPurple RGB\033[0m - Code: \\033[38;2;128;0;128m\n")
	fmt.Printf("\033[38;2;255;192;203mPink RGB\033[0m - Code: \\033[38;2;255;192;203m\n")
	fmt.Println()

	// Reset code
	fmt.Println("🔄 Reset Code:")
	fmt.Printf("\\033[0m - Resets all formatting\n")
	fmt.Println()

	// Demonstration using actual logging system
	fmt.Println("🔥 Live Logging System Demo:")
	logger := logging.WithPrefix("ColorDemo")
	logger.Debug("This is a DEBUG message")
	logger.Info("This is an INFO message")
	logger.Warn("This is a WARN message")
	logger.Error("This is an ERROR message")
	// Note: FATAL would exit the program, so we skip it

	// Color reference table
	fmt.Println("📋 Quick Reference Table:")
	fmt.Println("Standard Colors:    \\033[30-37m (black, red, green, yellow, blue, magenta, cyan, white)")
	fmt.Println("Bright Colors:      \\033[90-97m (bright variants)")
	fmt.Println("Background Colors:  \\033[40-47m (background versions)")
	fmt.Println("256 Colors:         \\033[38;5;Nm where N=0-255")
	fmt.Println("RGB Colors:         \\033[38;2;R;G;Bm where R,G,B=0-255")
	fmt.Println("Text Formatting:    \\033[1m(bold) \\033[2m(dim) \\033[3m(italic) \\033[4m(underline)")
	fmt.Println("Reset:              \\033[0m")

	fmt.Println()
	fmt.Println("=== End Color Reference ===")
}

func main() {
	colorDemo()
}