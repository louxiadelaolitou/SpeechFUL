import 'package:flutter_test/flutter_test.dart';
import 'package:hnuhole_mobile/src/comments/comment_http.dart';
import 'package:hnuhole_mobile/src/comments/comment_models.dart';
import 'package:hnuhole_mobile/src/posts/post_upload_models.dart';

String id(int value) =>
    '11111111-1111-4111-8111-${value.toString().padLeft(12, '0')}';

void main() {
  test(
    'polling skips confirmed upload; unknown upload retries identical manifest',
    () async {
      final intent = CommentIntent(
        commandId: 'AQEBAQEBAQEBAQEBAQEBAQ',
        taskId: id(1),
        operation: CommentOperation.create,
        binding: CommentBinding(postId: id(2), identityId: id(3)),
        content: CommentContent(
          body: 'material',
          image: CommentImage(
            blobId: id(4),
            selection: PostImageSelection.fromJson({
              'mediaId': id(5),
              'sha256': 'ab' * 32,
              'mimeType': 'image/png',
              'byteLength': 123,
              'width': 20,
              'height': 30,
            }),
          ),
        ),
      );
      var uploads = 0, fail = true;
      final api = HttpCommentApi(
        (method, path, {body, query, bytes, mime, snapshotRequest}) async =>
            path.contains('comment-commands')
            ? {
                'state': 'ACCEPTED',
                'operation': 'CREATE',
                'taskId': id(1),
                'attemptVersion': 1,
              }
            : {
                'taskId': id(1),
                'ownerIdentityId': id(3),
                'postId': id(2),
                'rootCommentId': null,
                'targetCommentId': null,
                'attemptVersion': 1,
                'state': 'ACCEPTED',
                'failureCode': null,
                'materialsAvailable': true,
                'content': null,
                'allowedActions': <String>[],
                'publishedCommentId': null,
              },
        (received, version) async {
          expect(identical(received, intent), isTrue);
          expect(version, 1);
          uploads++;
          if (fail) throw const CommentHttpFailure(null, 'TIMEOUT');
        },
      );
      await expectLater(api.query(intent), throwsA(isA<CommentHttpFailure>()));
      expect(uploads, 1);
      fail = false;
      await api.query(intent);
      await api.query(intent);
      await api.query(intent);
      expect(uploads, 2);
      api.clearUploadCache();
      await api.query(intent);
      expect(uploads, 3);
    },
  );
}
