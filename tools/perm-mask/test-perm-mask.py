import os

# Specify the path to the file you want to check
file_path = [
    r"D:\gem\YaeAchievement.exe",  # rwx
    r"D:\gem\Genshin Impact game\GenshinImpact_Data\Persistent\AssetBundles\blocks\15\34980066.blk",  # r
    r"D:\gem\Snap Hutao UIGF.json",  # rw
]


# Function to convert permission mask to a string like 'rwx'
def permission_mask_to_string(mask):
    permissions = ["---", "--x", "-w-", "-wx", "r--", "r-x", "rw-", "rwx"]
    return permissions[mask]


# Iterate through the file paths and check permissions
for path in file_path:
    try:
        # Get the file's mode (permissions) using os.stat
        file_mode = os.stat(path).st_mode

        # Extract permission bits for user, group, and others
        user_permissions = (file_mode & 0o700) >> 6
        group_permissions = (file_mode & 0o070) >> 3
        other_permissions = file_mode & 0o007

        # Print permissions for user, group, and others
        print(f"File: {path}")
        print(f"  User permissions:  {permission_mask_to_string(user_permissions)}")
        print(f"  Group permissions: {permission_mask_to_string(group_permissions)}")
        print(f"  Other permissions: {permission_mask_to_string(other_permissions)}")
    except FileNotFoundError:
        print(f"File not found: {path}")
    except Exception as e:
        print(f"Error processing file {path}: {e}")
