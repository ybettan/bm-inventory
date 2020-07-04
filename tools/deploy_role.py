import os
import utils
import argparse

parser = argparse.ArgumentParser()
parser.add_argument("--namespace", help='namespace to use', type=str, default='assisted-installer')
args = parser.parse_args()

def main():
    src_file = os.path.join(os.getcwd(), "deploy/roles/default_role.yaml")
    dst_file = os.path.join(os.getcwd(), "build/default_role.yaml")

    with open(src_file, "r") as src:
        with open(dst_file, "w+") as dst:
            data = src.read()
            data = data.replace('REPLACE_NAMESPACE', args.namespace)
            print("Deploying {}".format(dst_file))
            dst.write(data)

    utils.apply(dst_file)


if __name__ == "__main__":
    main()
