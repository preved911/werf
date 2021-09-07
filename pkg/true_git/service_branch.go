package true_git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/werf/logboek"
)

type SyncSourceWorktreeWithServiceBranchOptions struct {
	ServiceBranchPrefix string
	GlobIncludeList     []string
	GlobExcludeList     []string
}

func SyncSourceWorktreeWithServiceBranch(ctx context.Context, gitDir, sourceWorktreeDir, worktreeCacheDir, commit string, opts SyncSourceWorktreeWithServiceBranchOptions) (string, error) {
	var resultCommit string
	if err := withWorkTreeCacheLock(ctx, worktreeCacheDir, func() error {
		var err error
		if gitDir, err = filepath.Abs(gitDir); err != nil {
			return fmt.Errorf("bad git dir %s: %s", gitDir, err)
		}

		if worktreeCacheDir, err = filepath.Abs(worktreeCacheDir); err != nil {
			return fmt.Errorf("bad work tree cache dir %s: %s", worktreeCacheDir, err)
		}

		serviceWorktreeDir, err := prepareWorkTree(ctx, gitDir, worktreeCacheDir, commit, true)
		if err != nil {
			return fmt.Errorf("unable to prepare worktree for commit %v: %s", commit, err)
		}

		currentCommitPath := filepath.Join(worktreeCacheDir, "current_commit")
		if err := os.RemoveAll(currentCommitPath); err != nil {
			return fmt.Errorf("unable to remove %s: %s", currentCommitPath, err)
		}

		branchName := fmt.Sprintf("%s%s", opts.ServiceBranchPrefix, commit)
		resultCommit, err = syncWorktreeWithServiceWorktreeBranch(ctx, sourceWorktreeDir, serviceWorktreeDir, commit, branchName, opts.GlobIncludeList, opts.GlobExcludeList)
		if err != nil {
			return fmt.Errorf("unable to sync worktree with service branch %q: %s", branchName, err)
		}

		return nil
	}); err != nil {
		return "", err
	}

	return resultCommit, nil
}

func syncWorktreeWithServiceWorktreeBranch(ctx context.Context, sourceWorktreeDir, serviceWorktreeDir, sourceCommit, branchName string, globIncludeList, globExcludeList []string) (string, error) {
	serviceBranchHeadCommit, err := getOrPrepareServiceBranchHeadCommit(ctx, serviceWorktreeDir, sourceCommit, branchName)
	if err != nil {
		return "", fmt.Errorf("unable to get or prepare service branch head commit: %s", err)
	}

	if _, err := runGitCmd(ctx, []string{"checkout", branchName}, serviceWorktreeDir, runGitCmdOptions{}); err != nil {
		return "", fmt.Errorf("unable to checkout service branch: %s", err)
	}

	if err := revertExcludedChangesInServiceWorktreeIndex(ctx, sourceWorktreeDir, serviceWorktreeDir, sourceCommit, serviceBranchHeadCommit, globExcludeList); err != nil {
		return "", fmt.Errorf("unable to revert excluded changes in service worktree index: %q", err)
	}

	if err := addChangesToServiceWorktreeIndex(ctx, sourceWorktreeDir, serviceWorktreeDir, globIncludeList, globExcludeList); err != nil {
		return "", fmt.Errorf("unable to add changes to service worktree index: %s", err)
	}

	exist, err := checkNewChangesInServiceWorktreeIndex(ctx, serviceWorktreeDir)
	if err != nil {
		return "", fmt.Errorf("unable to check new changes in service worktree index: %s", err)
	}

	if !exist {
		return serviceBranchHeadCommit, nil
	}

	newCommit, err := commitNewChangesInServiceBranch(ctx, serviceWorktreeDir, branchName)
	if err != nil {
		return "", fmt.Errorf("unable to commit new changes in service branch: %s", err)
	}

	return newCommit, nil
}

func getOrPrepareServiceBranchHeadCommit(ctx context.Context, serviceWorktreeDir string, sourceCommit string, branchName string) (string, error) {
	var isServiceBranchExist bool
	output, err := runGitCmd(ctx, []string{"branch", "--list", branchName}, serviceWorktreeDir, runGitCmdOptions{})
	if err != nil {
		return "", err
	}
	isServiceBranchExist = output.Len() != 0

	if !isServiceBranchExist {
		if _, err := runGitCmd(ctx, []string{"checkout", "-b", branchName, sourceCommit}, serviceWorktreeDir, runGitCmdOptions{}); err != nil {
			return "", err
		}

		return sourceCommit, nil
	}

	output, err = runGitCmd(ctx, []string{"rev-parse", branchName}, serviceWorktreeDir, runGitCmdOptions{})
	if err != nil {
		return "", err
	}

	serviceBranchHeadCommit := strings.TrimSpace(output.String())
	return serviceBranchHeadCommit, nil
}

func revertExcludedChangesInServiceWorktreeIndex(ctx context.Context, sourceWorktreeDir string, serviceWorktreeDir string, sourceCommit string, serviceBranchHeadCommit string, globExcludeList []string) error {
	if len(globExcludeList) == 0 || serviceBranchHeadCommit == sourceCommit {
		return nil
	}

	gitDiffArgs := []string{
		"-c", "diff.renames=false",
		"-c", "core.quotePath=false",
		"diff",
		"--binary",
		serviceBranchHeadCommit, sourceCommit,
		"--",
	}
	gitDiffArgs = append(gitDiffArgs, globExcludeList...)

	diffOutput, err := runGitCmd(ctx, gitDiffArgs, sourceWorktreeDir, runGitCmdOptions{})
	if err != nil {
		return err
	}

	if len(diffOutput.Bytes()) == 0 {
		return nil
	}

	if _, err := runGitCmd(ctx, []string{"apply", "--binary", "--index"}, serviceWorktreeDir, runGitCmdOptions{stdin: diffOutput}); err != nil {
		return err
	}

	return nil
}

func addChangesToServiceWorktreeIndex(ctx context.Context, sourceWorktreeDir string, serviceWorktreeDir string, globIncludeList, globExcludeList []string) error {
	var pathSpecList []string
	for _, glob := range globIncludeList {
		pathSpecList = append(pathSpecList, ":"+glob)
	}
	for _, glob := range globExcludeList {
		pathSpecList = append(pathSpecList, ":!"+glob)
	}

	gitAddArgs := []string{
		"--work-tree",
		sourceWorktreeDir,
		"add",
		"--all",
		"--",
		".",
	}

	gitAddArgs = append(gitAddArgs, pathSpecList...)
	if _, err := runGitCmd(ctx, gitAddArgs, serviceWorktreeDir, runGitCmdOptions{}); err != nil {
		return err
	}

	return nil
}

func checkNewChangesInServiceWorktreeIndex(ctx context.Context, serviceWorktreeDir string) (bool, error) {
	gitDiffArgs := []string{
		"status",
		"-v",
		"-s",
	}

	output, err := runGitCmd(ctx, gitDiffArgs, serviceWorktreeDir, runGitCmdOptions{})
	if err != nil {
		return false, err
	}

	return len(output.Bytes()) != 0, nil
}

func commitNewChangesInServiceBranch(ctx context.Context, serviceWorktreeDir string, branchName string) (string, error) {
	gitArgs := []string{"-c", "user.email=werf@werf.io", "-c", "user.name=werf", "commit", "--no-verify", "-m", time.Now().String()}
	if _, err := runGitCmd(ctx, gitArgs, serviceWorktreeDir, runGitCmdOptions{}); err != nil {
		return "", err
	}

	output, err := runGitCmd(ctx, []string{"rev-parse", branchName}, serviceWorktreeDir, runGitCmdOptions{})
	if err != nil {
		return "", err
	}
	serviceNewCommit := strings.TrimSpace(output.String())

	if _, err := runGitCmd(ctx, []string{"checkout", "--force", "--detach", serviceNewCommit}, serviceWorktreeDir, runGitCmdOptions{}); err != nil {
		return "", err
	}

	return serviceNewCommit, nil
}

type runGitCmdOptions struct {
	stdin io.Reader
}

func runGitCmd(ctx context.Context, args []string, dir string, opts runGitCmdOptions) (*bytes.Buffer, error) {
	allArgs := append(getCommonGitOptions(), args...)
	cmd := exec.Command("git", allArgs...)
	cmd.Dir = dir

	if opts.stdin != nil {
		cmd.Stdin = opts.stdin
	}

	output := SetCommandRecordingLiveOutput(ctx, cmd)

	err := cmd.Run()

	cmdWithArgs := strings.Join(append([]string{cmd.Path, "-C " + dir}, cmd.Args[1:]...), " ")
	if debug() {
		logboek.Context(ctx).Debug().LogF("%s\n%s\n", cmdWithArgs, output)
	}

	if err != nil {
		return nil, err
	}

	return output, err
}

func debug() bool {
	return os.Getenv("WERF_DEBUG_TRUE_GIT") == "1"
}
