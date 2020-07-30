#!/usr/bin/env python

""",
Mocked Ruby based Sensu server. Connects to the RabbitMQ instance and sends
check requests as Sensu server would and waits for appropriate response.
Fails the response is not received at all or in invalid format.
"""

import click
import json
import pika
import signal
import sys

CI_TEST_RESULTS = {
    'test1': {'output': 'foo\n', 'status': 0},
    'test2': {'output': 'bar\n', 'status': 1},
    'test3': {'output': 'baz\n', 'status': 2},
    'standalone_check': {'output': 'foobar\n', 'status': 2}
}


def timeout_handler(signum, frame):
    print("Verification timed out")
    sys.exit(2)


@click.command()
@click.option('--rabbit-url', required=True,
              default='amqp://guest:guest@127.0.0.1:5672/%2fsensu')
@click.option('--timeout', type=int, required=True, default=10)
def main(rabbit_url, timeout):
    connection = pika.BlockingConnection(pika.URLParameters(rabbit_url))
    channel = connection.channel()

    signal.signal(signal.SIGALRM, timeout_handler)
    signal.alarm(timeout)

    try:
        hits = set()
        for method, properties, body in channel.consume('results'):
            channel.basic_ack(method.delivery_tag)
            result = json.loads(body)

            print(f"Verifying check result {result['check']['name']}")
            assert(result['check']['name'] in CI_TEST_RESULTS)
            print("Result name found in list of expected results.")

            for test in ("status", "output"):
                assert(CI_TEST_RESULTS[result['check']['name']][test] == result['check'][test])
                print(f"successful verification of {test} in result.")

            hits.add(result['check']['name'])
            if hits == set(CI_TEST_RESULTS.keys()):
                # end verification
                break
    except AssertionError as ex:
        print(f"Failed verification of {test} in result: {result}")
        sys.exit(1)
    else:
        sys.exit(0)
    finally:
        connection.close()


if __name__ == '__main__':
    main()
