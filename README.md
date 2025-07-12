# load-tester

A load testing tool for GraphQL.

Specify your request in a yaml file. You can use the `make` command to run the load test.

```
make run req=your-file.yaml
```


## An example file format
```.yaml
name: "Your mutation name"
description: "Your mutation description"

load:
  concurrency: 5 
  requests: 100 

logging:
  enabled: true
  file: "logs/your-mutation-name.csv"

url: "http://localhost:8894/graphql"

auth:
  header: "Authorization"
  value: "Bearer xxxxx"

query: |
  mutation YourMutationName($input: [YourMutationInput!]!) {
    yourMutationName(input: $input) {
      id
      result
    }
  }

variables:
  input:
    - id: "123"
      result: "success"
```

After running the test, you will get output like this:
```
=== results ===
total requests:     100
successful:         100 (100.00%)
failed:             0 (0.00%)
requests/sec:       21.86

=== latency ===
average:            306.374255ms
harmonic mean:      295.733821ms
minimum:            238.274417ms
maximum:            674.270834ms
range:              435.996417ms
standard deviation: 77.781134ms

=== percentiles ===
50th percentile:    285.397625ms
75th percentile:    291.79725ms
95th percentile:    546.543375ms
99th percentile:    670.649042ms
99.9th percentile:  674.270834ms

=== status codes ===
200: 100 (100.00%)
```

Within the file, you can use the following variables:

- `{{random.choice(list)}}` - Randomly select a value from a list
- `{{random.string(length)}}` - Generate a random string of a given length
- `{{random.int(min, max)}}` - Generate a random integer between a minimum and maximum value
- `{{random.float(min, max)}}` - Generate a random float between a minimum and maximum value
- `{{random.boolean()}}` - Generate a random boolean value
- `{{random.date(format)}}` - Generate a random date in a given format
- `{{random.time(format)}}` - Generate a random time in a given format

Here is an example with dynamic variables:
```.yaml
name: "Your mutation name"
description: "Your mutation description"

load:
  concurrency: 5 
  requests: 100 

logging:
  enabled: true
  file: "logs/your-mutation-name.csv"

url: "http://localhost:8894/graphql"

auth:
  header: "Authorization"
  value: "Bearer {{random.string(10)}}"

query: |
  mutation YourMutationName($input: [YourMutationInput!]!) {
    yourMutationName(input: $input) {
      id
      result
    }
  }

variables:
  input:
    - id: "{{random.string(10)}}"
      result: "{{random.choice(success,failure)}}"
```
