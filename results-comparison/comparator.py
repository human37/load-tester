#!/usr/bin/env python3
import json
import matplotlib.pyplot as plt
import numpy as np
from pathlib import Path
import os
import sys
import glob

try:
    import readline

    READLINE_AVAILABLE = True

    _completion_matches = []

    def path_completer(text, state):
        global _completion_matches

        if state == 0:
            _completion_matches = []
            try:
                if text.startswith("~"):
                    text = os.path.expanduser(text)

                if not text:
                    pattern = "*"
                    base_dir = "."
                elif text.endswith("/"):
                    pattern = text + "*"
                    base_dir = text
                else:
                    dirname = os.path.dirname(text)
                    basename = os.path.basename(text)

                    if dirname:
                        pattern = os.path.join(dirname, basename + "*")
                        base_dir = dirname
                    else:
                        pattern = basename + "*"
                        base_dir = "."

                raw_matches = glob.glob(pattern)

                for match in sorted(raw_matches):
                    if os.path.isdir(match):
                        _completion_matches.append(match + "/")
                    elif match.endswith(".json"):
                        _completion_matches.append(match)

                if not _completion_matches and raw_matches:
                    _completion_matches = sorted(raw_matches)

            except Exception as e:
                _completion_matches = []

        try:
            return _completion_matches[state]
        except IndexError:
            return None

    try:
        is_libedit = "libedit" in readline.__doc__ if readline.__doc__ else False

        readline.set_completer(path_completer)

        if is_libedit:
            try:
                readline.parse_and_bind("bind ^I rl_complete")
                print("✅ Tab completion enabled (libedit mode)!")
            except:
                readline.parse_and_bind("tab: complete")
                print("✅ Tab completion enabled (libedit fallback)!")
        else:
            readline.parse_and_bind("tab: complete")
            readline.parse_and_bind("set show-all-if-ambiguous on")
            readline.parse_and_bind("set completion-ignore-case on")
            print("✅ Tab completion enabled (GNU readline)!")

        readline.set_completer_delims(" \t\n")

        test_matches = path_completer("", 0)
        print(
            f"🔧 Completion test: Found {len(_completion_matches) if _completion_matches else 0} matches in current directory"
        )

    except Exception as e:
        print(f"⚠️ Tab completion setup failed: {e}")
        print("📝 Alternative: Use 'ls' command to browse directories")
        READLINE_AVAILABLE = False

except ImportError:
    READLINE_AVAILABLE = False
    print("⚠️ Readline not available - tab completion disabled")
    print("📝 Use 'ls' command to browse directories")


def enhanced_input_with_completion(prompt):
    if READLINE_AVAILABLE:
        try:
            return input(prompt)
        except EOFError:
            return None
    else:
        print("💡 Tip: Type 'ls' to see current directory contents")
        return input(prompt)


def list_available_paths(current_path=""):
    try:
        if not current_path:
            current_path = "."

        if current_path.startswith("~"):
            current_path = os.path.expanduser(current_path)

        if os.path.isdir(current_path):
            base_dir = current_path
            pattern = "*"
        else:
            base_dir = os.path.dirname(current_path) or "."
            pattern = os.path.basename(current_path) + "*"

        search_pattern = os.path.join(base_dir, pattern)
        matches = glob.glob(search_pattern)

        dirs = []
        json_files = []

        for match in sorted(matches):
            if os.path.isdir(match):
                dirs.append(match + "/")
            elif match.endswith(".json"):
                json_files.append(match)

        return dirs + json_files
    except Exception:
        return []


def parse_duration(duration_str):
    if "ms" in duration_str:
        return float(duration_str.replace("ms", ""))
    elif "s" in duration_str:
        return float(duration_str.replace("s", "")) * 1000
    return 0


def load_single_json(file_path):
    try:
        with open(file_path, "r") as f:
            data = json.load(f)

        return {
            "requests_per_second": data["summary"]["requests_per_second"],
            "average_latency": parse_duration(data["latency"]["average"]),
            "p50_latency": parse_duration(data["percentiles"]["p50"]),
            "p75_latency": parse_duration(data["percentiles"]["p75"]),
            "p95_latency": parse_duration(data["percentiles"]["p95"]),
            "p99_latency": parse_duration(data["percentiles"]["p99"]),
            "p999_latency": parse_duration(data["percentiles"]["p999"]),
            "success_rate": data["summary"]["success_rate_percent"],
            "num_files": 1,
            "source": os.path.basename(file_path),
        }
    except Exception as e:
        print(f"Error loading {file_path}: {e}")
        return None


def load_directory_average(directory_path):
    directory = Path(directory_path)

    if not directory.exists() or not directory.is_dir():
        return None

    json_files = list(directory.glob("*.json"))
    if not json_files:
        print(f"No JSON files found in {directory_path}")
        return None

    results = []
    for json_file in json_files:
        data = load_single_json(json_file)
        if data:
            results.append(data)

    if not results:
        return None

    avg_data = {
        "requests_per_second": sum(r["requests_per_second"] for r in results)
        / len(results),
        "average_latency": sum(r["average_latency"] for r in results) / len(results),
        "p50_latency": sum(r["p50_latency"] for r in results) / len(results),
        "p75_latency": sum(r["p75_latency"] for r in results) / len(results),
        "p95_latency": sum(r["p95_latency"] for r in results) / len(results),
        "p99_latency": sum(r["p99_latency"] for r in results) / len(results),
        "p999_latency": sum(r["p999_latency"] for r in results) / len(results),
        "success_rate": sum(r["success_rate"] for r in results) / len(results),
        "num_files": len(results),
        "source": f"{directory.name} (avg of {len(results)} files)",
    }

    return avg_data


def validate_path(path):
    path_obj = Path(path)
    if not path_obj.exists():
        return False, "Path does not exist"

    if path_obj.is_file():
        if not path.endswith(".json"):
            return False, "File must be a JSON file"
        return True, "valid_file"
    elif path_obj.is_dir():
        json_files = list(path_obj.glob("*.json"))
        if not json_files:
            return False, "Directory contains no JSON files"
        return True, "valid_directory"
    else:
        return False, "Path is neither a file nor directory"


def get_user_input(prompt_label):
    while True:
        print(f"\n{prompt_label}")
        print("Enter either:")
        print("  - Path to a JSON file (e.g., results/test.json)")
        print("  - Path to a directory containing JSON files (e.g., results/)")
        print("  - Type 'ls' or 'list' to see current directory contents")
        print("  - Type 'ls <path>' to see contents of a specific directory")
        print("  - Type 'quit' to exit")

        if READLINE_AVAILABLE:
            print("  💡 Use TAB for auto-completion of paths!")
        else:
            print("  💡 Use 'ls' command to browse available files/directories")

        try:
            user_input = enhanced_input_with_completion(
                f"\n{prompt_label} path: "
            ).strip()
        except EOFError:
            print("\nExiting...")
            sys.exit(0)
        except Exception as e:
            print(f"Input error: {e}")
            continue

        if user_input is None:
            print("\nExiting...")
            sys.exit(0)

        if user_input.lower() == "quit":
            print("Exiting...")
            sys.exit(0)

        if user_input.lower() in ["ls", "list"]:
            available = list_available_paths(".")
            if available:
                print("\nAvailable files and directories:")
                for item in available[:20]:
                    print(f"  {item}")
                if len(available) > 20:
                    print(f"  ... and {len(available) - 20} more")
            else:
                print("No files or directories found")
            continue

        if user_input.lower().startswith("ls "):
            path_to_list = user_input[3:].strip()
            available = list_available_paths(path_to_list)
            if available:
                print(f"\nAvailable in '{path_to_list}':")
                for item in available[:20]:
                    print(f"  {item}")
                if len(available) > 20:
                    print(f"  ... and {len(available) - 20} more")
            else:
                print(f"No files or directories found in '{path_to_list}'")
            continue

        if not user_input:
            print("Please enter a valid path.")
            continue

        expanded_path = os.path.expanduser(user_input)

        is_valid, result = validate_path(expanded_path)

        if is_valid:
            print(f"✓ Valid {result.replace('valid_', '')}: {expanded_path}")
            return expanded_path, result
        else:
            print(f"✗ Error: {result}")
            print(f"  Path checked: {expanded_path}")

            dirname = os.path.dirname(expanded_path) or "."
            if os.path.exists(dirname):
                similar = list_available_paths(dirname)
                if similar:
                    print("  💡 Available in this directory:")
                    for item in similar[:5]:
                        print(f"    {item}")


def load_data(path, path_type):
    if path_type == "valid_file":
        return load_single_json(path)
    elif path_type == "valid_directory":
        return load_directory_average(path)
    return None


def create_comparison_graph(data_a, data_b, label_a, label_b):
    fig, ((ax1, ax2), (ax3, ax4)) = plt.subplots(2, 2, figsize=(16, 12))
    fig.suptitle(
        f"Load Test Comparison: {label_a} vs {label_b}", fontsize=16, fontweight="bold"
    )

    scenarios = [label_a, label_b]
    colors = ["#2ECC40", "#0074D9"]  # green and blue

    rps_values = [data_a["requests_per_second"], data_b["requests_per_second"]]
    bars1 = ax1.bar(scenarios, rps_values, color=colors, alpha=0.8)
    ax1.set_ylabel("Requests per Second")
    ax1.set_title("Throughput Comparison")
    ax1.grid(axis="y", alpha=0.3)

    for bar, value in zip(bars1, rps_values):
        ax1.text(
            bar.get_x() + bar.get_width() / 2,
            bar.get_height() + max(rps_values) * 0.01,
            f"{value:.2f}",
            ha="center",
            va="bottom",
            fontweight="bold",
        )

    latency_values = [data_a["average_latency"], data_b["average_latency"]]
    bars2 = ax2.bar(scenarios, latency_values, color=colors, alpha=0.8)
    ax2.set_ylabel("Average Latency (ms)")
    ax2.set_title("Average Response Time")
    ax2.grid(axis="y", alpha=0.3)

    for bar, value in zip(bars2, latency_values):
        ax2.text(
            bar.get_x() + bar.get_width() / 2,
            bar.get_height() + max(latency_values) * 0.01,
            f"{value:.1f}ms",
            ha="center",
            va="bottom",
            fontweight="bold",
        )

    percentiles = ["P50", "P75", "P95", "P99", "P99.9"]
    values_a = [
        data_a["p50_latency"],
        data_a["p75_latency"],
        data_a["p95_latency"],
        data_a["p99_latency"],
        data_a["p999_latency"],
    ]
    values_b = [
        data_b["p50_latency"],
        data_b["p75_latency"],
        data_b["p95_latency"],
        data_b["p99_latency"],
        data_b["p999_latency"],
    ]

    x = np.arange(len(percentiles))
    width = 0.35

    bars3a = ax3.bar(
        x - width / 2, values_a, width, label=label_a, color=colors[0], alpha=0.8
    )
    bars3b = ax3.bar(
        x + width / 2, values_b, width, label=label_b, color=colors[1], alpha=0.8
    )

    ax3.set_ylabel("Latency (ms)")
    ax3.set_title("Latency Percentiles Comparison")
    ax3.set_xticks(x)
    ax3.set_xticklabels(percentiles)
    ax3.legend()
    ax3.grid(axis="y", alpha=0.3)

    metrics = ["RPS", "Avg Lat", "P95", "P99"]
    differences = [
        (
            (data_b["requests_per_second"] - data_a["requests_per_second"])
            / data_a["requests_per_second"]
            * 100
        ),
        (
            (data_b["average_latency"] - data_a["average_latency"])
            / data_a["average_latency"]
            * 100
        ),
        ((data_b["p95_latency"] - data_a["p95_latency"]) / data_a["p95_latency"] * 100),
        ((data_b["p99_latency"] - data_a["p99_latency"]) / data_a["p99_latency"] * 100),
    ]

    colors_diff = ["green" if d > 0 else "red" for d in differences]
    bars4 = ax4.bar(metrics, differences, color=colors_diff, alpha=0.7)
    ax4.set_ylabel("Percentage Difference (%)")
    ax4.set_title(f"Performance Difference ({label_b} vs {label_a})")
    ax4.axhline(y=0, color="black", linestyle="-", alpha=0.3)
    ax4.grid(axis="y", alpha=0.3)

    for bar, value in zip(bars4, differences):
        ax4.text(
            bar.get_x() + bar.get_width() / 2,
            bar.get_height() + (1 if value > 0 else -3),
            f"{value:+.1f}%",
            ha="center",
            va="bottom" if value > 0 else "top",
            fontweight="bold",
        )

    summary_text = f"""
    {label_a}: {data_a['source']} ({data_a['num_files']} file{'s' if data_a['num_files'] > 1 else ''})
    {label_b}: {data_b['source']} ({data_b['num_files']} file{'s' if data_b['num_files'] > 1 else ''})
    """

    fig.text(0.02, 0.02, summary_text.strip(), fontsize=10, alpha=0.7)

    plt.tight_layout()

    output_file = f"comparison_graph_{datetime.now().strftime('%Y%m%d_%H%M%S')}.png"
    plt.savefig(output_file, dpi=300, bbox_inches="tight")
    print(f"\n📊 Graph saved as: {output_file}")
    plt.show()


def print_detailed_comparison(data_a, data_b, label_a, label_b):
    print("\n" + "=" * 80)
    print("DETAILED LOAD TEST COMPARISON")
    print("=" * 80)
    print(f"{'Metric':<20} {label_a:<25} {label_b:<25} {'Difference':<10}")
    print("-" * 80)

    metrics = [
        ("Requests/sec", "requests_per_second", ".2f"),
        ("Avg Latency (ms)", "average_latency", ".1f"),
        ("P50 Latency (ms)", "p50_latency", ".1f"),
        ("P75 Latency (ms)", "p75_latency", ".1f"),
        ("P95 Latency (ms)", "p95_latency", ".1f"),
        ("P99 Latency (ms)", "p99_latency", ".1f"),
        ("P99.9 Latency (ms)", "p999_latency", ".1f"),
        ("Success Rate (%)", "success_rate", ".2f"),
    ]

    for metric_name, key, fmt in metrics:
        val_a = data_a[key]
        val_b = data_b[key]
        diff = ((val_b - val_a) / val_a * 100) if val_a != 0 else 0

        print(f"{metric_name:<20} {val_a:<25{fmt}} {val_b:<25{fmt}} {diff:+.1f}%")

    print("=" * 80)


def main():
    print("🚀 Load Test Results Comparator")
    print("=" * 50)

    path_a, type_a = get_user_input("Input A")
    data_a = load_data(path_a, type_a)

    if not data_a:
        print("❌ Failed to load data from Input A")
        return

    print(f"✅ Successfully loaded Input A: {data_a['source']}")

    path_b, type_b = get_user_input("Input B")
    data_b = load_data(path_b, type_b)

    if not data_b:
        print("❌ Failed to load data from Input B")
        return

    print(f"✅ Successfully loaded Input B: {data_b['source']}")

    print(f"\nCurrent labels:")
    print(f"  A: {data_a['source']}")
    print(f"  B: {data_b['source']}")

    use_custom = (
        enhanced_input_with_completion("\nWould you like to use custom labels? (y/n): ")
        .strip()
        .lower()
    )

    if use_custom == "y":
        label_a = (
            enhanced_input_with_completion("Enter label for A: ").strip()
            or data_a["source"]
        )
        label_b = (
            enhanced_input_with_completion("Enter label for B: ").strip()
            or data_b["source"]
        )
    else:
        label_a = data_a["source"]
        label_b = data_b["source"]

    print("\n🔄 Generating comparison...")
    print_detailed_comparison(data_a, data_b, label_a, label_b)
    create_comparison_graph(data_a, data_b, label_a, label_b)
    print("\n✅ Comparison complete!")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n\n👋 Goodbye!")
    except Exception as e:
        print(f"\n❌ An error occurred: {e}")
