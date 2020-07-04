import os
import utils
import argparse

parser = argparse.ArgumentParser()
parser.add_argument("--namespace", help='namespace to use', type=str, default='assisted-installer')
args = parser.parse_args()


def main():
    src_file = os.path.join(os.getcwd(), "deploy/s3/scality-configmap.yaml")
    dst_file = os.path.join(os.getcwd(), "build/scality-configmap.yaml")
    scality_url = "http://cloudserver-front:8000"
    with open(src_file, "r") as src:
        with open(dst_file, "w+") as dst:
            data = src.read()
            data = data.replace('REPLACE_NAMESPACE', args.namespace)
            data = data.replace('REPLACE_URL', scality_url)
            print("Deploying {}".format(dst_file))
            dst.write(data)

    utils.apply(dst_file)


if __name__ == "__main__":
    main()
